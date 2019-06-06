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

	"github.com/umbracle/minimal/helper/derivesha"
	"github.com/umbracle/minimal/helper/hex"

	"github.com/armon/go-metrics"

	"github.com/hashicorp/go-hclog"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/crypto"
	"github.com/umbracle/minimal/network/transport/rlpx"
	"github.com/umbracle/minimal/rlp"
	"github.com/umbracle/minimal/types"
	"golang.org/x/crypto/sha3"
)

const (
	defaultMaxHeaderFetch  = 192
	defaultMaxBlockFetch   = 128
	defaultMaxReceiptFetch = 256
)

const (
	softResponseLimit = 2 * 1024 * 1024
	estHeaderRlpSize  = 500
)

// eth protocol message codes
const (
	// Protocol messages belonging to eth/62
	StatusMsg          = 0x00
	NewBlockHashesMsg  = 0x01
	TxMsg              = 0x02
	GetBlockHeadersMsg = 0x03
	BlockHeadersMsg    = 0x04
	GetBlockBodiesMsg  = 0x05
	BlockBodiesMsg     = 0x06
	NewBlockMsg        = 0x07

	// Protocol messages belonging to eth/63
	GetNodeDataMsg = 0x0d
	NodeDataMsg    = 0x0e
	GetReceiptsMsg = 0x0f
	ReceiptsMsg    = 0x10
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

// Ethereum is the protocol for etheruem
type Ethereum struct {
	logger   hclog.Logger
	conn     net.Conn
	sendLock sync.Mutex

	backend *Backend

	status     *Status // status of the remote peer
	blockchain *blockchain.Blockchain

	// pending objects
	pending map[messageType]*pending

	// header data
	HeaderHash   types.Hash
	HeaderDiff   *big.Int
	HeaderNumber *big.Int
	headerLock   sync.Mutex

	sendHeader rlpx.Header
	recvHeader rlpx.Header

	// peer *PeerConnection
	peerID string
}

// NotifyMsg notifies that there is a new block
type NotifyMsg struct {
	Block *types.Block
	// Peer  *PeerConnection
	Diff *big.Int
}

// NewEthereumProtocol creates the ethereum protocol
func NewEthereumProtocol(peerID string, logger hclog.Logger, conn net.Conn, blockchain *blockchain.Blockchain) *Ethereum {
	e := &Ethereum{
		logger:     logger,
		peerID:     peerID,
		conn:       conn,
		blockchain: blockchain,
		sendHeader: make([]byte, rlpx.HeaderSize),
		recvHeader: make([]byte, rlpx.HeaderSize),
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
	ProtocolVersion uint32
	NetworkID       uint64
	TD              *big.Int
	CurrentBlock    types.Hash
	GenesisBlock    types.Hash
}

// getBlockHeadersData represents a block header query.
type getBlockHeadersData struct {
	Origin  interface{}
	Amount  uint64
	Skip    uint64
	Reverse bool
}

func (e *Ethereum) writeMsg(code int, data interface{}) error {
	e.sendLock.Lock()
	defer e.sendLock.Unlock()

	r, err := rlp.EncodeToBytes(data)
	if err != nil {
		return err
	}

	e.sendHeader.Encode(uint16(code), uint32(len(r)))
	if _, err := e.conn.Write(e.sendHeader[:]); err != nil {
		return err
	}
	if _, err := e.conn.Write(r); err != nil {
		return err
	}
	return nil
}

// RequestHeadersByNumber fetches a batch of blocks' headers based on the number of an origin block.
func (e *Ethereum) RequestHeadersByNumber(number uint64, amount uint64, skip uint64, reverse bool) error {
	return e.writeMsg(GetBlockHeadersMsg, &getBlockHeadersData{Origin: number, Amount: amount, Skip: skip, Reverse: reverse})
}

// RequestHeadersByHash fetches a batch of blocks' headers based on the hash of an origin block.
func (e *Ethereum) RequestHeadersByHash(origin types.Hash, amount uint64, skip uint64, reverse bool) error {
	return e.writeMsg(GetBlockHeadersMsg, &getBlockHeadersData{Origin: origin, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse})
}

// RequestBodies fetches a batch of blocks' bodies based on a set of hashes.
func (e *Ethereum) RequestBodies(hashes []types.Hash) error {
	return e.writeMsg(GetBlockBodiesMsg, hashes)
}

// RequestNodeData fetches a batch of arbitrary data from a node's known state.
func (e *Ethereum) RequestNodeData(hashes []types.Hash) error {
	return e.writeMsg(GetNodeDataMsg, hashes)
}

// RequestReceipts fetches a batch of transaction receipts from a remote node.
func (e *Ethereum) RequestReceipts(hashes []types.Hash) error {
	return e.writeMsg(GetReceiptsMsg, hashes)
}

// SendNewBlock propagates an entire block to a remote peer.
func (e *Ethereum) SendNewBlock(block *types.Block, td *big.Int) error {
	return e.writeMsg(NewBlockMsg, &newBlockData{Block: block, TD: td})
}

// SendNewHash propagates a new block by hash and number
func (e *Ethereum) SendNewHash(hash types.Hash, number uint64) error {
	return e.writeMsg(NewBlockHashesMsg, []*announcement{
		{hash, number},
	})
}

func (e *Ethereum) sendStatus(status *Status) error {
	return e.writeMsg(StatusMsg, status)
}

// Info implements the handler interface
func (e *Ethereum) Info() (map[string]interface{}, error) {
	var msg map[string]interface{}
	err := mapstructure.Decode(e.status, &msg)
	return msg, err
}

func (e *Ethereum) readStatus(localStatus *Status) error {
	if _, err := e.conn.Read(e.recvHeader[:]); err != nil {
		return err
	}
	if code := e.recvHeader.MsgType(); code != StatusMsg {
		return fmt.Errorf("Message code is not statusMsg but %d", code)
	}
	buf := make([]byte, e.recvHeader.Length())

	_, err := e.conn.Read(buf)
	if err != nil {
		return err
	}
	if err := rlp.DecodeBytes(buf, &e.status); err != nil {
		return err
	}

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

	var errors error
	for i := 0; i < 2; i++ {
		select {
		case err := <-errr:
			if err != nil {
				errors = multierror.Append(errors, err)
			}
		case <-time.After(5 * time.Second):
			return fmt.Errorf("ethereum protocol handshake timeout")
		}
	}
	if errors != nil {
		return errors
	}

	// handshake was correct, start to listen for packets
	go e.listen()
	return nil
}

func (e *Ethereum) readMsg() (*rlpx.Message, error) {
	if _, err := e.conn.Read(e.recvHeader[:]); err != nil {
		return nil, err
	}

	// TODO, reuse buffer
	buf := make([]byte, e.recvHeader.Length())
	if _, err := e.conn.Read(buf); err != nil {
		return nil, err
	}

	msg := &rlpx.Message{
		Code:    uint64(e.recvHeader.MsgType()),
		Payload: bytes.NewReader(buf),
	}
	return msg, nil
}

func (e *Ethereum) listen() {
	for {
		msg, err := e.readMsg()
		if err != nil {
			e.logger.Warn("failed to read msg", err.Error())
			break
		}

		if err := e.HandleMsg(*msg); err != nil {
			e.logger.Warn("failed to handle msg", err.Error())
			break
		}
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

	return nil, false, fmt.Errorf("bad")
}

// HandleMsg handles a message from ethereum
func (e *Ethereum) HandleMsg(msg rlpx.Message) error {
	metrics.IncrCounterWithLabels([]string{"minimal", "protocol", "ethereum63", "msg"}, float32(1.0), []metrics.Label{{Name: "Code", Value: strconv.Itoa(int(msg.Code))}})

	code := msg.Code

	switch {
	case code == StatusMsg:
		return fmt.Errorf("Status msg not expected after handshake")

	case code == GetBlockHeadersMsg:
		defer metrics.MeasureSince([]string{"minimal", "protocol", "ethereum63", "getHeaders"}, time.Now())

		var query getBlockHeadersData
		err := msg.Decode(&query)
		if err != nil {
			return err
		}

		var origin *types.Header
		var ok bool

		start, isHash, err := parseOrigin(query.Origin)
		if err != nil {
			panic(err)
		}
		if isHash {
			origin, ok = e.blockchain.GetHeaderByHash(start.(types.Hash))
		} else {
			origin, ok = e.blockchain.GetHeaderByNumber(start.(uint64))
		}

		if !ok {
			return e.sendBlockHeaders([]*types.Header{})
		}

		headers := []*types.Header{origin}
		bytes := 0

		skip := int64(query.Skip) + 1

		dir := int64(1)
		if query.Reverse {
			dir = -1
		}

		for len(headers) < int(query.Amount) && bytes < softResponseLimit && len(headers) < defaultMaxHeaderFetch {
			block := int64(origin.Number)
			block = block + (dir)*skip

			if block < 0 {
				break
			}
			origin, ok = e.blockchain.GetHeaderByNumber(uint64(block))
			if !ok {
				break
			}

			headers = append(headers, origin)
			bytes += estHeaderRlpSize
		}

		return e.sendBlockHeaders(headers)

	case code == BlockHeadersMsg:
		var headers []*types.Header
		if err := msg.Decode(&headers); err != nil {
			return err
		}
		e.Headers(headers)

	case code == GetBlockBodiesMsg:
		defer metrics.MeasureSince([]string{"minimal", "protocol", "ethereum63", "getBodies"}, time.Now())

		var hashes []types.Hash
		if err := msg.Decode(&hashes); err != nil {
			return err
		}

		// need to use the encoded version to keep track of the byte size
		bodies := []*types.Body{}
		for i := 0; i < len(hashes) && len(bodies) < defaultMaxBlockFetch; i++ {
			hash := hashes[i]

			body, ok := e.blockchain.GetBodyByHash(hash)
			if ok {
				bodies = append(bodies, body)
			}
		}
		return e.sendBlockBodies(bodies)

	case code == BlockBodiesMsg:
		var bodies []*types.Body
		if err := msg.Decode(&bodies); err != nil {
			return err
		}
		e.Bodies(bodies)

	case code == GetNodeDataMsg:
		// TODO. send

	case code == NodeDataMsg:
		var data [][]byte

		if err := msg.Decode(&data); err != nil {
			panic(err)
		}
		e.Data(data)

	case code == GetReceiptsMsg:
		defer metrics.MeasureSince([]string{"minimal", "ethereum", "getReceipts"}, time.Now())

		var hashes []types.Hash
		if err := msg.Decode(&hashes); err != nil {
			return err
		}

		// need to use the encoded version to keep track of the byte size
		receipts := [][]*types.Receipt{}
		bytes := 0

		for i := 0; i < len(hashes) && bytes < softResponseLimit && len(receipts) < defaultMaxReceiptFetch; i++ {
			hash := hashes[i]

			res := e.blockchain.GetReceiptsByHash(hash)
			if res == nil {
				header, ok := e.blockchain.GetHeaderByHash(hash)
				if !ok || header.ReceiptsRoot != types.EmptyRootHash {
					continue
				}
			}
			receipts = append(receipts, res)
		}
		return e.sendReceipts(receipts)

	case code == ReceiptsMsg:
		var receipts [][]*types.Receipt
		if err := msg.Decode(&receipts); err != nil {
			return err
		}
		e.Receipts(receipts)

	case code == NewBlockHashesMsg:
		// We are notified about the new peer blocks.
		if err := e.handleNewBlockHashesMsg(msg); err != nil {
			return err
		}

	case code == NewBlockMsg:
		// This is the last block of the peer
		if err := e.handleNewBlockMsg(msg); err != nil {
			return err
		}

	case code == TxMsg:
		// TODO: deliver

	default:
		return fmt.Errorf("Message code %d not found", code)
	}

	return nil
}

type announcement struct {
	Hash   types.Hash
	Number uint64
}

func (e *Ethereum) handleNewBlockHashesMsg(msg rlpx.Message) error {

	// fmt.Printf("===> NOTIFY (%s) HASHES\n", e.peerID)

	var announces []*announcement
	if err := msg.Decode(&announces); err != nil {
		panic(err)
	}

	/*
		for _, i := range announces {
			e.backend.watcher.notifyHash(i.Hash, i.Number)
		}
	*/

	return nil
}

func (e *Ethereum) handleNewBlockMsg(msg rlpx.Message) error {
	var request newBlockData
	if err := msg.Decode(&request); err != nil {
		return err
	}

	/*
		a := request.Block.Number()
		b := request.TD.String()
		c := request.Block.Difficulty().String()
	*/

	// trueTD := new(big.Int).Sub(request.TD, request.Block.Difficulty())
	// trueTD := request.TD

	// fmt.Printf("===> NOTIFY (%s) Block: %d Difficulty %s. Total: %s\n", e.peerID, a.Uint64(), c, b)

	/*
		if trueTD.Cmp(e.HeaderDiff) > 0 {
			e.headerLock.Lock()
			e.HeaderDiff = request.TD
			e.HeaderHash = request.Block.Hash() // NOTE. not sure about this thing of not addedd yet
			e.headerLock.Unlock()

			// go e.backend.notifyNewData(&NotifyMsg{Block: request.Block, Peer: e.peer, Diff: trueTD})
		} else {
			fmt.Println("-- NO UPDATE --")
			fmt.Println(trueTD.Int64())
			fmt.Println(e.HeaderDiff.Int64())
		}
	*/

	return nil
}

// fetchHeight returns the header of the head hash of the peer
func (e *Ethereum) fetchHeight(ctx context.Context) (*types.Header, error) {
	head := e.HeaderHash

	header, err := e.RequestHeaderByHashSync(ctx, head)
	if err != nil {
		return nil, err
	}
	if header == nil {
		return nil, fmt.Errorf("header not found")
	}
	if header.Hash() != head {
		return nil, fmt.Errorf("returned hash is not the correct one")
	}
	return header, nil
}

// sendBlockHeaders sends a batch of block headers to the remote peer.
func (e *Ethereum) sendBlockHeaders(headers []*types.Header) error {
	return e.writeMsg(BlockHeadersMsg, headers)
}

func (e *Ethereum) sendBlockBodies(bodies []*types.Body) error {
	return e.writeMsg(BlockBodiesMsg, bodies)
}

func (e *Ethereum) sendReceipts(receipts [][]*types.Receipt) error {
	return e.writeMsg(ReceiptsMsg, receipts)
}

// -- handlers --

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
	ack chan AckMessage
}

var (
	// TODO, there are still a couple of DAO validations to perform
	// on the ethash consensus algorithm
	daoBlock            = uint64(1920000)
	daoChallengeTimeout = 15 * time.Second
	daoForkBlockExtra   = hex.MustDecodeHex("0x64616f2d686172642d666f726b")
)

// ValidateDAOBlock queries the DAO block
func (e *Ethereum) ValidateDAOBlock() error {
	ctx, cancel := context.WithTimeout(context.Background(), daoChallengeTimeout)
	defer cancel()

	header, ok, err := e.RequestHeaderSync(ctx, daoBlock)
	if err != nil {
		// We accept peers that return empty queries
		if err != errorEmptyQuery {
			return err
		}
	}

	// If it returns nothing it means the node does not have the dao block yet
	if ok {
		if !bytes.Equal(header.ExtraData, daoForkBlockExtra[:]) {
			return fmt.Errorf("Dao extra data does not match")
		}
	}
	return nil
}

// RequestHeaderByHashSync requests a header by hash synchronously
func (e *Ethereum) RequestHeaderByHashSync(ctx context.Context, hash types.Hash) (*types.Header, error) {
	ack := make(chan AckMessage, 1)
	e.setHandler(ctx, headerMsg, hash.String(), ack)

	if err := e.RequestHeadersByHash(hash, 1, 0, false); err != nil {
		return nil, err
	}
	resp := <-ack
	if !resp.Completed() {
		return nil, fmt.Errorf("failed")
	}

	response := resp.Result.([]*types.Header)
	return response[0], nil
}

func (e *Ethereum) RequestHeaderSync(ctx context.Context, origin uint64) (*types.Header, bool, error) {
	hash := strconv.Itoa(int(origin))

	ack := make(chan AckMessage, 1)
	e.setHandler(ctx, headerMsg, hash, ack)

	if err := e.RequestHeadersByNumber(origin, 1, 0, false); err != nil {
		return nil, false, err
	}
	resp := <-ack
	if !resp.Completed() {
		return nil, false, resp.Error
	}

	response := resp.Result.([]*types.Header)
	if len(response) > 1 {
		return nil, false, fmt.Errorf("too many responses")
	}
	if len(response) == 0 {
		return nil, false, nil
	}
	return response[0], true, nil
}

// RequestHeadersRangeSync requests a range of headers syncronously
func (e *Ethereum) RequestHeadersRangeSync(ctx context.Context, origin uint64, count uint64) ([]*types.Header, error) {
	return e.RequestHeadersSync(ctx, origin, 0, count)
}

// RequestHeadersSync requests headers and waits for the response
func (e *Ethereum) RequestHeadersSync(ctx context.Context, origin uint64, skip uint64, count uint64) ([]*types.Header, error) {
	hash := strconv.Itoa(int(origin))

	ack := make(chan AckMessage, 1)
	e.setHandler(ctx, headerMsg, hash, ack)

	if err := e.RequestHeadersByNumber(origin, count, skip, false); err != nil {
		return nil, err
	}
	resp := <-ack
	if !resp.Completed() {
		return nil, fmt.Errorf("failed")
	}

	response := resp.Result.([]*types.Header)
	return response, nil
}

// RequestReceiptsSync requests receipts and waits for the response
func (e *Ethereum) RequestReceiptsSync(ctx context.Context, hash string, hashes []types.Hash) ([][]*types.Receipt, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	ack := make(chan AckMessage, 1)
	e.setHandler(ctx, receiptsMsg, hash, ack)

	if err := e.RequestReceipts(hashes); err != nil {
		return nil, err
	}
	resp := <-ack
	if !resp.Completed() {
		return nil, fmt.Errorf("failed")
	}

	// TODO. handle malformed response in the receipts
	response := resp.Result.([][]*types.Receipt)
	return response, nil
}

// RequestBodiesSync requests bodies and waits for the response
func (e *Ethereum) RequestBodiesSync(ctx context.Context, hash string, hashes []types.Hash) ([]*types.Body, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	ack := make(chan AckMessage, 1)
	e.setHandler(ctx, bodyMsg, hash, ack)

	if err := e.RequestBodies(hashes); err != nil {
		return nil, err
	}
	resp := <-ack
	if !resp.Completed() {
		return nil, fmt.Errorf("failed")
	}

	// TODO. handle malformed response in the bodies
	response := resp.Result.([]*types.Body)

	res := []*types.Body{}
	for _, r := range response {
		res = append(res, &types.Body{Transactions: r.Transactions, Uncles: r.Uncles})
	}

	return res, nil
}

// RequestNodeDataSync requests node data and waits for the response
func (e *Ethereum) RequestNodeDataSync(ctx context.Context, hashes []types.Hash) ([][]byte, error) {
	ack := make(chan AckMessage, 1)
	e.setHandler(ctx, dataMsg, hashes[0].String(), ack)

	if err := e.RequestNodeData(hashes); err != nil {
		return nil, err
	}
	resp := <-ack
	if !resp.Completed() {
		return nil, fmt.Errorf("failed")
	}

	return resp.Result.([][]byte), nil
}

func (e *Ethereum) setHandler(ctx context.Context, typ messageType, key string, ack chan AckMessage) error {
	queue, ok := e.pending[typ]
	if !ok {
		panic("internal. message type not found")
	}

	queue.add(key, &callback{ack})

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

// -- downloader --

// Headers receives the headers
func (e *Ethereum) Headers(headers []*types.Header) {
	if len(headers) != 0 {
		// request the header by number registers the number of the peer
		number := strconv.Itoa(int(headers[0].Number))
		if e.consumeHandler(number, headerMsg, headers) {
			return
		}
		// reqest the header by hash registers the hash of the peer
		hash := headers[0].Hash().String()
		if e.consumeHandler(hash, headerMsg, headers) {
			return
		}
	}
	// Neither hash nor number found. Consume empty response.
	e.consumeHandler("", headerMsg, nil)
}

// Receipts receives the receipts
func (e *Ethereum) Receipts(receipts [][]*types.Receipt) {
	origin := ""
	if len(receipts) != 0 {
		origin = derivesha.CalcReceiptRoot(receipts[0]).String()
	}
	if e.consumeHandler(origin, receiptsMsg, receipts) {
		return
	}
}

// Bodies receives the bodies
func (e *Ethereum) Bodies(bodies []*types.Body) {
	origin := ""
	if len(bodies) != 0 {
		first := bodies[0]

		hash := derivesha.CalcTxsRoot(first.Transactions)
		origin = encodeHash(derivesha.CalcUncleRoot(first.Uncles), hash).String()
	}
	if e.consumeHandler(origin, bodyMsg, bodies) {
		return
	}
}

// Data receives the node state data
func (e *Ethereum) Data(data [][]byte) {
	origin := ""
	if len(data) != 0 {
		origin = hex.EncodeToHex(crypto.Keccak256(data[0]))
	}
	if e.consumeHandler(origin, dataMsg, data) {
		return
	}
}

func encodeHash(x types.Hash, y types.Hash) types.Hash {
	hw := sha3.NewLegacyKeccak256()
	if _, err := hw.Write(x.Bytes()); err != nil {
		panic(err)
	}
	if _, err := hw.Write(y.Bytes()); err != nil {
		panic(err)
	}

	var h types.Hash
	hw.Sum(h[:0])
	return h
}
