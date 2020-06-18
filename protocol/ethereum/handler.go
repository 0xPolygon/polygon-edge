package ethereum

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/mitchellh/mapstructure"

	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/helper/keccak"
	"github.com/0xPolygon/minimal/network"
	"github.com/umbracle/fastrlp"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/network/transport/rlpx"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
)

var (
	errEmpty   = fmt.Errorf("empty")
	errTimeout = fmt.Errorf("timeout")
)

const (
	softResponseLimit = 2 * 1024 * 1024
)

type ethMessage int16

const (
	StatusMsg          ethMessage = 0x00
	NewBlockHashesMsg             = 0x01
	TxMsg                         = 0x02
	GetBlockHeadersMsg            = 0x03
	BlockHeadersMsg               = 0x04
	GetBlockBodiesMsg             = 0x05
	BlockBodiesMsg                = 0x06
	NewBlockMsg                   = 0x07
	GetNodeDataMsg                = 0x0d
	NodeDataMsg                   = 0x0e
	GetReceiptsMsg                = 0x0f
	ReceiptsMsg                   = 0x10
)

type messageType int

const (
	headerMsg messageType = iota
	bodyMsg
	receiptsMsg
	dataMsg
)

type pending struct {
	sync.Mutex

	handler map[string]*callback
	pending int
}

var (
	errorTimeoutQuery = fmt.Errorf("timeout")
	errorEmptyQuery   = fmt.Errorf("empty response")
)

func newPending() *pending {
	return &pending{
		handler: map[string]*callback{},
		pending: 0,
	}
}

func (p *pending) add(id string, c *callback) {
	p.Lock()
	defer p.Unlock()

	p.handler[id] = c
	p.pending++
}

func (p *pending) freeHandlersLocked() {
	for origin, i := range p.handler {
		select {
		case i.ack <- AckMessage{Error: errorEmptyQuery}:
		default:
		}
		delete(p.handler, origin)
	}
}

const empty = ""

func (p *pending) consume(id string) (*callback, bool) {
	p.Lock()
	defer p.Unlock()

	if id == "" {
		if p.pending != 0 {
			p.pending--
			if p.pending == 0 {
				p.freeHandlersLocked()
			}
		}
		return nil, false
	}

	handler, ok := p.handler[id]
	if !ok {
		return nil, false
	}

	delete(p.handler, id)
	p.pending--
	if p.pending == 0 {
		p.freeHandlersLocked()
	}

	return handler, true
}

// Blockchain interface are the methods required by the handler
type Blockchain interface {
	GetReceiptsByHash(types.Hash) []*types.Receipt
	GetHeaderByHash(hash types.Hash) (*types.Header, bool)
	GetHeaderByNumber(number uint64) (*types.Header, bool)
	GetBodyByHash(types.Hash) (*types.Body, bool)
}

// Ethereum is the protocol for etheruem
type Ethereum struct {
	logger hclog.Logger
	conn   net.Conn

	backend *Backend

	status     *Status // status of the remote peer
	blockchain Blockchain

	// pending objects
	pending map[messageType]*pending

	// header data
	HeaderHash   types.Hash
	HeaderDiff   *big.Int
	HeaderNumber *big.Int
	headerLock   sync.Mutex

	in, out halfConn

	session network.Session
	peerID  string
}

type halfConn struct {
	sync.Mutex

	buf    []byte
	header rlpx.Header
}

func (h *halfConn) encode(code uint16, length int) {
	h.header.Encode(code, uint32(length))
}

// NotifyMsg notifies that there is a new block
type NotifyMsg struct {
	Block *types.Block
	Diff  *big.Int
}

// NewEthereumProtocol creates the ethereum protocol
func NewEthereumProtocol(session network.Session, peerID string, logger hclog.Logger, conn net.Conn, blockchain *blockchain.Blockchain) *Ethereum {
	e := &Ethereum{
		session:    session,
		logger:     logger,
		peerID:     peerID,
		conn:       conn,
		blockchain: blockchain,
	}
	e.pending = map[messageType]*pending{
		headerMsg:   newPending(),
		bodyMsg:     newPending(),
		receiptsMsg: newPending(),
		dataMsg:     newPending(),
	}
	return e
}

func (e *Ethereum) Header() types.Hash {
	return e.HeaderHash
}

// Status is the object for the status message.
type Status struct {
	ProtocolVersion uint64
	NetworkID       uint64
	TD              *big.Int
	CurrentBlock    types.Hash
	GenesisBlock    types.Hash
}

var statusParserPool fastrlp.ParserPool

func (s *Status) UnmarshalRLP(buf []byte) error {
	p := statusParserPool.Get()
	defer statusParserPool.Put(p)

	v, err := p.Parse(buf)
	if err != nil {
		return err
	}
	elems, err := v.GetElems()
	if err != nil {
		return err
	}
	if len(elems) != 5 {
		return fmt.Errorf("bad length, expected 5 items but found %d", len(elems))
	}

	if s.ProtocolVersion, err = elems[0].GetUint64(); err != nil {
		return err
	}
	if s.NetworkID, err = elems[1].GetUint64(); err != nil {
		return err
	}
	s.TD = new(big.Int)
	if err := elems[2].GetBigInt(s.TD); err != nil {
		return err
	}
	if err = elems[3].GetHash(s.CurrentBlock[:]); err != nil {
		return err
	}
	if err = elems[4].GetHash(s.GenesisBlock[:]); err != nil {
		return err
	}
	return nil
}

var statusArenaPool fastrlp.ArenaPool

func (s *Status) MarshalRLP() []byte {
	a := statusArenaPool.Get()

	v := a.NewArray()
	v.Set(a.NewUint(s.ProtocolVersion))
	v.Set(a.NewUint(s.NetworkID))
	v.Set(a.NewBigInt(s.TD))
	v.Set(a.NewBytes(s.CurrentBlock[:]))
	v.Set(a.NewBytes(s.GenesisBlock[:]))

	dst := v.MarshalTo(nil)
	statusArenaPool.Put(a)
	return dst
}

func (e *Ethereum) sendStatus(status *Status) error {
	buf := status.MarshalRLP()
	return e.writeRaw(StatusMsg, buf)
}

// Info implements the handler interface
func (e *Ethereum) Info() (map[string]interface{}, error) {
	var msg map[string]interface{}
	err := mapstructure.Decode(e.status, &msg)
	return msg, err
}

func (e *Ethereum) readStatus(localStatus *Status) error {
	buf, code, err := e.readMsg()
	if err != nil {
		return err
	}
	if ethMessage(code) != StatusMsg {
		return fmt.Errorf("Message code is not statusMsg but %d", code)
	}

	var ss Status
	if err := ss.UnmarshalRLP(buf); err != nil {
		panic(err)
	}
	e.status = &ss

	// Validate status

	if e.status.NetworkID != localStatus.NetworkID {
		return fmt.Errorf("Network id does not match. Found %d but expected %d", e.status.NetworkID, localStatus.NetworkID)
	}
	if e.status.GenesisBlock != localStatus.GenesisBlock {
		return fmt.Errorf("Genesis block does not match. Found %s but expected %s", e.status.GenesisBlock.String(), localStatus.GenesisBlock.String())
	}
	if int(e.status.ProtocolVersion) != int(localStatus.ProtocolVersion) {
		return fmt.Errorf("Protocol version does not match. Found %d but expected %d", int(e.status.ProtocolVersion), int(localStatus.ProtocolVersion))
	}

	e.HeaderHash = e.status.CurrentBlock
	e.HeaderDiff = e.status.TD

	return nil
}

// Close the protocol
func (e *Ethereum) Close() error {
	return nil
}

func (e *Ethereum) Status() *Status {
	return e.status
}

// Init starts the protocol
func (e *Ethereum) Init(status *Status) error {
	errr := make(chan error, 2)

	go func() {
		errr <- e.readStatus(status)
	}()

	go func() {
		errr <- e.sendStatus(status)
	}()

	for i := 0; i < 2; i++ {
		select {
		case err := <-errr:
			if err != nil {
				return err
			}
		case <-time.After(5 * time.Second):
			return fmt.Errorf("ethereum protocol handshake timeout")
		}
	}

	// handshake was correct, start to listen for packets
	go e.listen()
	return nil
}

func extendByteSlice(b []byte, needLen int) []byte {
	b = b[:cap(b)]
	if n := needLen - cap(b); n > 0 {
		b = append(b, make([]byte, n)...)
	}
	return b[:needLen]
}

func (e *Ethereum) readMsg() ([]byte, uint16, error) {
	if e.in.header == nil {
		e.in.header = make([]byte, rlpx.HeaderSize)
	}
	if _, err := e.conn.Read(e.in.header[:]); err != nil {
		return nil, 0, err
	}
	e.in.buf = extendByteSlice(e.in.buf, int(e.in.header.Length()))
	if _, err := e.conn.Read(e.in.buf); err != nil {
		return nil, 0, err
	}
	return e.in.buf, e.in.header.MsgType(), nil
}

func (e *Ethereum) writeRaw(code ethMessage, b []byte) error {
	e.out.Lock()
	defer e.out.Unlock()

	if e.out.header == nil {
		e.out.header = make([]byte, rlpx.HeaderSize)
	}

	// write the header
	e.out.encode(uint16(code), len(b))

	// write to the connection
	if _, err := e.conn.Write(e.out.header); err != nil {
		return err
	}
	if _, err := e.conn.Write(b); err != nil {
		return err
	}
	return nil
}

func (e *Ethereum) writeRLP(code uint16, v *fastrlp.Value) error {
	e.out.Lock()
	defer e.out.Unlock()

	if e.out.header == nil {
		e.out.header = make([]byte, rlpx.HeaderSize)
	}

	// convert rlp data to bytes
	e.out.buf = v.MarshalTo(e.out.buf[:0])

	// write the header
	e.out.encode(code, len(e.out.buf))

	// write to the connection
	if _, err := e.conn.Write(e.out.header); err != nil {
		return err
	}
	if _, err := e.conn.Write(e.out.buf); err != nil {
		return err
	}
	return nil
}

var (
	handlers = map[uint16]func(*Ethereum, *fastrlp.Parser, *fastrlp.Value) error{
		// responses from other peers
		BlockHeadersMsg: (*Ethereum).handleHeader,
		BlockBodiesMsg:  (*Ethereum).handleBodies,
		ReceiptsMsg:     (*Ethereum).handleReceipts,
		NodeDataMsg:     (*Ethereum).handlerNodeData,

		// requests from other peers
		GetBlockHeadersMsg: (*Ethereum).handleGetHeader,
		GetBlockBodiesMsg:  (*Ethereum).handleGetBodies,
		GetReceiptsMsg:     (*Ethereum).handleGetReceipts,
		GetNodeDataMsg:     (*Ethereum).handleGetNodeData,

		// notifications
		TxMsg:             (*Ethereum).handleTxnMessage,
		NewBlockHashesMsg: (*Ethereum).handleNewBlockHashesMsg,
		NewBlockMsg:       (*Ethereum).handleNewBlockMsg,
	}
)

func (e *Ethereum) dispatchMsg(code uint16, p *fastrlp.Parser, v *fastrlp.Value) error {
	handler, ok := handlers[code]
	if !ok {
		return fmt.Errorf("handler not found for code %d", code)
	}
	if err := handler(e, p, v); err != nil {
		return err
	}
	return nil
}

func (e *Ethereum) fetchBlock(hash types.Hash) (*types.Block, error) {
	headers, err := e.requestHeaderByNumber2(0, &hash, 1, 0, false)
	if err != nil {
		return nil, err
	}

	header := headers[0]
	block := &types.Block{Header: header}

	// check if its necessary to download receipts and transactions
	if hasBody(*header) {
		body, err := e.requestBody(hash, types.BytesToHash(xor(header.TxRoot, header.Sha3Uncles)))
		if err != nil {
			return nil, err
		}
		block.Transactions = body.Transactions
		block.Uncles = body.Uncles
	}

	return block, nil
}

func (e *Ethereum) handleNewBlockMsg(p *fastrlp.Parser, v *fastrlp.Value) error {
	return e.backend.announceNewBlock(e, p, v)
}

func (e *Ethereum) handleNewBlockHashesMsg(p *fastrlp.Parser, v *fastrlp.Value) error {
	// expects an array of structs {hash and number}
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	// prevalidate the input
	for _, i := range elems {
		tuple, err := i.GetElems()
		if err != nil {
			return err
		}
		if err := validateHash(tuple[0]); err != nil {
			return err
		}
		if err := validateUint64(tuple[1]); err != nil {
			return err
		}
	}
	return e.backend.announceNewBlocksHashes(e, v)
}

func validateHash(v *fastrlp.Value) error {
	if v.Type() != fastrlp.TypeBytes {
		return fmt.Errorf("bad")
	}
	if v.Len() != 32 {
		return fmt.Errorf("bad length")
	}
	return nil
}

func validateUint64(v *fastrlp.Value) error {
	if v.Type() != fastrlp.TypeBytes {
		return fmt.Errorf("bad")
	}
	if v.Len() > 8 {
		return fmt.Errorf("bad length")
	}
	return nil
}

func (e *Ethereum) handleTxnMessage(p *fastrlp.Parser, v *fastrlp.Value) error {
	return nil
}

func (e *Ethereum) handleHeader(p *fastrlp.Parser, v *fastrlp.Value) error {
	// receives a header from some peer
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	// no headers found
	if len(elems) == 0 {
		e.consumeHandler(empty, headerMsg, nil)
		return nil
	}

	// decode the first header to search the beacon
	var header types.Header
	if err := header.UnmarshalRLP(p, elems[0]); err != nil {
		return err
	}

	// check if the number is found
	if handler := e.getHandler(strconv.Itoa(int(header.Number)), headerMsg); handler != nil {
		return handler(p, v)
	}

	hash := keccak.DefaultKeccakPool.Get()
	kk := hash.WriteRlp(nil, elems[0])
	keccak.DefaultKeccakPool.Put(hash)

	if handler := e.getHandler(hex.EncodeToHex(kk), headerMsg); handler != nil {
		return handler(p, v)
	}
	return nil
}

func (e *Ethereum) handleBodies(p *fastrlp.Parser, v *fastrlp.Value) error {
	bodies, err := v.GetElems()
	if err != nil {
		return err
	}
	if len(bodies) == 0 {
		e.consumeHandler(empty, bodyMsg, nil)
		return nil
	}

	// In the bodies we use the txroot + the sha3uncles of the first body as a beacon
	// Note that we always ask for entries that have either one of those things

	items, err := bodies[0].GetElems()
	if err != nil {
		return err
	}
	if len(items) != 2 {
		return fmt.Errorf("bad entry in bodies")
	}
	txns, err := items[0].GetElems()
	if err != nil {
		return err
	}

	txnsRoot := calculateRoot(p, txns, types.EmptyRootHash)

	var unclesRoot types.Hash
	hash := keccak.DefaultKeccakPool.Get()
	hash.WriteRlp(unclesRoot[:0], items[1])
	keccak.DefaultKeccakPool.Put(hash)

	// Use xor to encode both roots in a single hash
	root := xor(txnsRoot, unclesRoot)

	if handler := e.getHandler(hex.EncodeToHex(root), bodyMsg); handler != nil {
		return handler(p, v)
	}
	return nil
}

func (e *Ethereum) handleReceipts(p *fastrlp.Parser, v *fastrlp.Value) error {
	// same thing applies to bodies but with a bit more playing
	receipts, err := v.GetElems()
	if err != nil {
		return err // failed to parse
	}
	if len(receipts) == 0 {
		e.consumeHandler(empty, receiptsMsg, nil)
		return nil
	}

	elems, err := receipts[0].GetElems()
	if err != nil {
		return err
	}

	beacon := calculateRoot(p, elems, types.EmptyRootHash)
	if handler := e.getHandler(beacon.String(), receiptsMsg); handler != nil {
		return handler(p, v)
	}

	return nil
}

func (e *Ethereum) handlerNodeData(p *fastrlp.Parser, v *fastrlp.Value) error {
	return nil
}

func validateHashesReq(elems []*fastrlp.Value) error {
	for _, elem := range elems {
		buf, err := elem.Bytes()
		if err != nil {
			return err
		}
		if len(buf) != 32 {
			return fmt.Errorf("incorrect size")
		}
	}
	return nil
}

func minUint64(i, j uint64) uint64 {
	if i < j {
		return i
	}
	return j
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}

const maxReceiptsAmount = 100

var receiptsArenaPool fastrlp.ArenaPool

func (e *Ethereum) handleGetReceipts(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}
	// validate that all the elements are hashes
	if err := validateHashesReq(elems); err != nil {
		return err
	}

	// limit the number of receipts
	amount := min(len(elems), maxReceiptsAmount)

	ar := receiptsArenaPool.Get()
	res := ar.NewArray()

	for _, elem := range elems[:amount] {
		hash := types.BytesToHash(elem.Raw())

		receipts := e.blockchain.GetReceiptsByHash(hash)
		if res == nil {
			header, ok := e.blockchain.GetHeaderByHash(hash)
			if !ok || header.ReceiptsRoot != types.EmptyRootHash {
				continue
			}
		}

		vv := ar.NewArray()
		for _, receipt := range receipts {
			vv.Set(receipt.MarshalWith(ar))
		}
		res.Set(vv)
	}

	e.writeRLP(ReceiptsMsg, res)
	receiptsArenaPool.Put(ar)
	return nil
}

const maxBodiesAmount = 100

var bodyArenaPool fastrlp.ArenaPool

func (e *Ethereum) handleGetBodies(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}
	// validate that all the elements are hashes
	if err := validateHashesReq(elems); err != nil {
		return err
	}

	ar := bodyArenaPool.Get()
	res := ar.NewArray()

	// limit the number of elements
	amount := min(len(elems), maxBodiesAmount)

	// TODO, limit size
	for _, elem := range elems[:amount] {
		body, ok := e.blockchain.GetBodyByHash(types.BytesToHash(elem.Raw()))
		if ok {
			res.Set(body.MarshalWith(ar))
		} else {
			res.Set(ar.NewNullArray())
		}
	}

	e.writeRLP(BlockBodiesMsg, res)
	bodyArenaPool.Put(ar)
	return nil
}

func (e *Ethereum) handleGetNodeData(p *fastrlp.Parser, v *fastrlp.Value) error {
	// TODO
	return nil
}

const maxHeadersAmount = 190

var headerArenaPool fastrlp.ArenaPool

func (e *Ethereum) handleGetHeader(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}
	amount, err := elems[1].GetUint64()
	if err != nil {
		return err
	}
	skipU, err := elems[2].GetUint64()
	if err != nil {
		return err
	}
	reverse, err := elems[3].GetBool()
	if err != nil {
		return err
	}

	ar := headerArenaPool.Get()

	var origin *types.Header
	var ok bool

	buf, err := elems[0].Bytes()
	if err != nil {
		return err
	}
	if len(buf) == 32 {
		// hash
		origin, ok = e.blockchain.GetHeaderByHash(types.BytesToHash(buf))
	} else {
		// number
		num, err := elems[0].GetUint64()
		if err != nil {
			return err
		}
		origin, ok = e.blockchain.GetHeaderByNumber(num)
	}

	if !ok {
		// origin is unknown, return an empty array set
		e.writeRLP(BlockHeadersMsg, ar.NewNullArray())
		headerArenaPool.Put(ar)
		return nil
	}

	skip := int64(skipU) + 1
	dir := int64(1)
	if reverse {
		dir = -1
	}

	// clap to max amount
	if amount > maxHeadersAmount {
		amount = maxHeadersAmount
	}

	res := ar.NewArray()
	res.Set(origin.MarshalWith(ar)) // add origin

	count := uint64(1)
	for count < amount {
		block := int64(origin.Number)
		block = block + (dir)*skip

		if block < 0 {
			break
		}
		origin, ok = e.blockchain.GetHeaderByNumber(uint64(block))
		if !ok {
			break
		}
		count++
		res.Set(origin.MarshalWith(ar))
	}

	e.writeRLP(BlockHeadersMsg, res)
	headerArenaPool.Put(ar)
	return nil
}

// defPool is the default rlp parser pool to use if no specific parser if found in the msg pools
var defPool fastrlp.ParserPool

func (e *Ethereum) listen() {
	for {
		// read the message
		buf, code, err := e.readMsg()
		if err != nil {
			e.logger.Error("failed to read msg", err.Error())
			return
		}

		p := defPool.Get()
		v, err := p.Parse(buf)
		if err != nil {
			e.logger.Error("failed to parse msg", err.Error())
			continue
		}

		// dispatch the message in a go routine
		go func() {
			if err := e.dispatchMsg(code, p, v); err != nil {
				e.logger.Error("failed to handle msg", err.Error())
			}
			// return the parser to the pool
			defPool.Put(p)
		}()
	}
}

type newBlockData struct {
	Block *types.Block
	TD    *big.Int
}

func parseOrigin(i interface{}) (interface{}, bool, error) {
	buf, ok := i.([]byte)
	if !ok {
		return nil, false, fmt.Errorf("could not convert to bytes")
	}

	size := len(buf)
	if size == 32 {
		// hash
		return types.BytesToHash(buf), true, nil
	} else if size <= 8 {
		// number (uint64). TODO. Optimize
		aux := make([]byte, 8)
		copy(aux[8-size:], buf[:])

		return binary.BigEndian.Uint64(aux[:]), false, nil
	}

	return nil, false, fmt.Errorf("bad 3")
}

func reqHeadersQuery2(dst []byte, originN uint64, originH *types.Hash, amount, skip uint64, reverse bool) []byte {
	a := reqHeadersPool.Get()

	v := a.NewArray()
	if originH != nil {
		v.Set(a.NewCopyBytes(originH.Bytes())) // origin is a hash
	} else {
		v.Set(a.NewUint(originN)) // origin is a number
	}
	v.Set(a.NewUint(amount))  // amount
	v.Set(a.NewUint(skip))    // skip
	v.Set(a.NewBool(reverse)) // reverse

	dst = v.MarshalTo(dst)
	reqHeadersPool.Put(a)
	return dst
}

// var kPool keccakPool

var defaultArenaPool fastrlp.ArenaPool

func (e *Ethereum) requestBody(hash types.Hash, beacon types.Hash) (*types.Body, error) {
	ack := make(chan AckMessage, 1)

	body := new(types.Body)
	e.setHandler2(context.Background(), bodyMsg, beacon.String(), ack, func(p *fastrlp.Parser, v *fastrlp.Value) error {
		// We already know this data is valid because otherwise there would not have been any match in the beacon

		items, err := v.GetElems()
		if err != nil {
			return err
		}
		if len(items) != 1 {
			return fmt.Errorf("there should be only one")
		}

		elems, err := items[0].GetElems()
		if err != nil {
			return err
		}
		if len(elems) != 2 {
			return fmt.Errorf("expected 2")
		}

		txns, err := elems[0].GetElems()
		if err != nil {
			return err
		}

		transactions := []*types.Transaction{}
		for _, i := range txns {
			txn := new(types.Transaction)
			if err := txn.UnmarshalRLP(p, i); err != nil {
				return err
			}
			transactions = append(transactions, txn)
		}

		uncles, err := elems[1].GetElems()
		if err != nil {
			return err
		}

		unclesX := []*types.Header{}
		for _, i := range uncles {
			uncle := new(types.Header)
			if err := uncle.UnmarshalRLP(p, i); err != nil {
				return err
			}
			unclesX = append(unclesX, uncle)
		}

		body.Transactions = transactions
		body.Uncles = unclesX

		return nil
	})

	a := defaultArenaPool.Get()
	defer defaultArenaPool.Put(a)

	v := a.NewArray()
	v.Set(a.NewCopyBytes(hash.Bytes()))

	if err := e.writeRLP(GetBlockBodiesMsg, v); err != nil {
		return nil, err
	}

	resp := <-ack
	if resp.Error != nil {
		return nil, resp.Error
	}
	return body, nil
}

func (e *Ethereum) requestHeaderByNumber2(originN uint64, originH *types.Hash, amount, skip uint64, reverse bool) ([]*types.Header, error) {
	buf := reqHeadersQuery2(nil, originN, originH, amount, skip, reverse)

	var headers []*types.Header
	ack := make(chan AckMessage, 1)

	var beacon string
	if originH != nil {
		beacon = originH.String()
	} else {
		beacon = strconv.Itoa(int(originN))
	}

	e.setHandler2(context.Background(), headerMsg, beacon, ack, func(p *fastrlp.Parser, v *fastrlp.Value) error {
		elems, err := v.GetElems()
		if err != nil {
			return err
		}
		if len(elems) == 0 {
			return errEmpty
		}
		if len(elems) > int(amount) {
			return fmt.Errorf("returned more elements than requested")
		}

		hash := keccak.DefaultKeccakPool.Get()
		defer keccak.DefaultKeccakPool.Put(hash)

		headers = make([]*types.Header, len(elems))
		for indx := range headers {
			header := new(types.Header)

			// unmarshal the header
			if err := header.UnmarshalRLP(p, elems[indx]); err != nil {
				return err
			}
			// decode the hash
			buf := p.Raw(elems[indx])

			hash.Write(buf)
			hash.Sum(header.Hash[:0])
			hash.Reset()

			//keccak.Hash(header.SHash[:], buf)
			//keccak.Reset()

			headers[indx] = header
		}

		// TODO, validate response
		return nil
	})
	if err := e.writeRaw(GetBlockHeadersMsg, buf); err != nil {
		return nil, err
	}

	resp := <-ack
	if resp.Error != nil {
		return nil, resp.Error
	}
	return headers, nil
}

func (e *Ethereum) requestHeaderByNumber(num uint64) (*types.Header, error) {
	headers, err := e.requestHeaderByNumber2(num, nil, 1, 0, false)
	if err != nil {
		return nil, err
	}
	return headers[0], nil
}

func (e *Ethereum) fetchHeight2() (*types.Header, error) {
	headers, err := e.requestHeaderByNumber2(0, &e.HeaderHash, 1, 0, false)
	if err != nil {
		return nil, err
	}
	return headers[0], nil
}

// -- handlers --

type rawFunc func(p *fastrlp.Parser, v *fastrlp.Value) error

// AckMessage is the ack message
type AckMessage struct {
	Error  error
	Result interface{}
}

// TODO: AckMessage with two fields one for error and another
// to handle empty responses. The api functions could return now
// three fields (result, ok, error) to represent empty data.

// Completed returns true if there is a value in the response
func (a *AckMessage) Completed() bool {
	return a.Error == nil
}

type callback struct {
	raw rawFunc
	ack chan AckMessage
}

var (
	daoBlock            = uint64(1920000)
	daoChallengeTimeout = 15 * time.Second
	daoForkBlockExtra   = hex.MustDecodeHex("0x64616f2d686172642d666f726b")
)

// ValidateDAOBlock queries the DAO block
func (e *Ethereum) ValidateDAOBlock() error {
	header, err := e.requestHeaderByNumber(daoBlock)
	if err != nil {
		if err != errorEmptyQuery {
			return err
		}
	}

	// If it returns nothing it means the node does not have the dao block yet
	if err == nil {
		if !bytes.Equal(header.ExtraData, daoForkBlockExtra[:]) {
			return fmt.Errorf("Dao extra data does not match")
		}
	}
	return nil
}

// DoRequest is an adhoc method to do Skeleton requests
func (e *Ethereum) DoRequest(req *Request, q *Queue3) error {

	var msg messageType // find a better way for this
	switch req.typ {
	case Headers:
		msg = headerMsg
	case Bodies:
		msg = bodyMsg
	case Receipts:
		msg = receiptsMsg
	}
	// still need to get the code of the request

	f := func(p *fastrlp.Parser, v *fastrlp.Value) error {
		return q.DeliverJob(req, p, v)
	}

	ack := make(chan AckMessage, 1)
	// fmt.Printf("Handler: %s %s\n", e.peerID, req.origin)
	e.setHandler2(context.Background(), msg, req.origin, ack, f)

	var code uint16
	switch req.typ {
	case Headers:
		code = GetBlockHeadersMsg
	case Bodies:
		code = GetBlockBodiesMsg
	case Receipts:
		code = GetReceiptsMsg
	}

	if err := e.writeRaw(ethMessage(code), req.buf); err != nil {
		return err
	}

	resp := <-ack
	if resp.Error != nil {
		return resp.Error
	}
	return nil
}

func (e *Ethereum) setHandler2(ctx context.Context, typ messageType, key string, ack chan AckMessage, raw rawFunc) {
	queue, ok := e.pending[typ]
	if !ok {
		panic("internal. message type not found")
	}
	queue.add(key, &callback{raw: raw, ack: ack})

	go func() {
		select {
		case <-time.After(5 * time.Second):
		}

		if _, ok := queue.consume(key); !ok {
			// The key has already been consumed
			return
		}

		select {
		case ack <- AckMessage{Error: errorTimeoutQuery}:
		default:
		}
	}()
}

func (e *Ethereum) setHandler(ctx context.Context, typ messageType, key string, ack chan AckMessage) error {
	queue, ok := e.pending[typ]
	if !ok {
		panic("internal. message type not found")
	}

	queue.add(key, &callback{ack: ack})

	go func() {
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}

		if _, ok := queue.consume(key); !ok {
			// The key has already been consumed
			return
		}

		select {
		case ack <- AckMessage{Error: errorTimeoutQuery, Result: nil}:
		default:
		}
	}()

	return nil
}

func (e *Ethereum) getHandler(origin string, typ messageType) rawFunc {
	queue, ok := e.pending[typ]
	if !ok {
		panic("internal. message type not found")
	}
	callback, ok := queue.consume(origin)
	if !ok {
		return nil
	}

	return func(p *fastrlp.Parser, v *fastrlp.Value) error {
		err := callback.raw(p, v)
		if err != nil {
			callback.ack <- AckMessage{Error: err}
		} else {
			callback.ack <- AckMessage{Error: nil}
		}
		return err
	}
}

func (e *Ethereum) consumeHandler(origin string, typ messageType, result interface{}) bool {
	queue, ok := e.pending[typ]
	if !ok {
		panic("internal. message type not found")
	}

	callback, ok := queue.consume(origin)
	if !ok {
		return false
	}

	// notify its over
	select {
	case callback.ack <- AckMessage{Result: result}:
	default:
	}
	return true
}

func xor(a, b types.Hash) []byte {
	// TODO; avoid allocation
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		buf[i] = a[i] ^ b[i]
	}
	return buf
}
