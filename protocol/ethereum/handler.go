package ethereum

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/mitchellh/mapstructure"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/armon/go-metrics"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/rlp"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/network/transport/rlpx"
	"golang.org/x/crypto/sha3"
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
		case i.ack <- AckMessage{Complete: false}:
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
	conn     net.Conn
	sendLock sync.Mutex

	backend *Backend

	status     *Status // status of the remote peer
	blockchain *blockchain.Blockchain

	// pending objects
	pending map[messageType]*pending

	// header data
	HeaderHash   common.Hash
	HeaderDiff   *big.Int
	HeaderNumber *big.Int
	headerLock   sync.Mutex

	sendHeader rlpx.Header
	recvHeader rlpx.Header

	peer *PeerConnection
}

// NotifyMsg notifies that there is a new block
type NotifyMsg struct {
	Block *types.Block
	Peer  *PeerConnection
	Diff  *big.Int
}

// NewEthereumProtocol creates the ethereum protocol
func NewEthereumProtocol(conn net.Conn, blockchain *blockchain.Blockchain) *Ethereum {
	e := &Ethereum{
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

func (e *Ethereum) Header() common.Hash {
	return e.HeaderHash
}

// Status is the object for the status message.
type Status struct {
	ProtocolVersion uint32
	NetworkID       uint64
	TD              *big.Int
	CurrentBlock    common.Hash
	GenesisBlock    common.Hash
}

// getBlockHeadersData represents a block header query.
type getBlockHeadersData struct {
	Origin  hashOrNumber
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
	return e.writeMsg(GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Number: number}, Amount: amount, Skip: skip, Reverse: reverse})
}

// RequestHeadersByHash fetches a batch of blocks' headers based on the hash of an origin block.
func (e *Ethereum) RequestHeadersByHash(origin common.Hash, amount uint64, skip uint64, reverse bool) error {
	return e.writeMsg(GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Hash: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse})
}

// RequestBodies fetches a batch of blocks' bodies based on a set of hashes.
func (e *Ethereum) RequestBodies(hashes []common.Hash) error {
	return e.writeMsg(GetBlockBodiesMsg, hashes)
}

// RequestNodeData fetches a batch of arbitrary data from a node's known state.
func (e *Ethereum) RequestNodeData(hashes []common.Hash) error {
	return e.writeMsg(GetNodeDataMsg, hashes)
}

// RequestReceipts fetches a batch of transaction receipts from a remote node.
func (e *Ethereum) RequestReceipts(hashes []common.Hash) error {
	return e.writeMsg(GetReceiptsMsg, hashes)
}

// SendNewBlock propagates an entire block to a remote peer.
func (e *Ethereum) SendNewBlock(block *types.Block, td *big.Int) error {
	return e.writeMsg(NewBlockMsg, &newBlockData{Block: block, TD: td})
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

func (e *Ethereum) listen() {
	for {
		if _, err := e.conn.Read(e.recvHeader[:]); err != nil {
			panic(err)
		}

		buf := make([]byte, e.recvHeader.Length())
		if _, err := e.conn.Read(buf); err != nil {
			panic(err)
		}

		msg := rlpx.Message{
			Code:    uint64(e.recvHeader.MsgType()),
			Payload: bytes.NewReader(buf),
		}
		if err := e.HandleMsg(msg); err != nil {
			panic(err)
		}
	}
}

type newBlockData struct {
	Block *types.Block
	TD    *big.Int
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
		if query.Origin.IsHash() {
			origin, ok = e.blockchain.GetHeaderByHash(query.Origin.Hash)
		} else {
			origin, ok = e.blockchain.GetHeaderByNumber(big.NewInt(int64(query.Origin.Number)))
		}

		if !ok {
			return e.sendBlockHeaders([]*types.Header{})
		}

		headers := []*types.Header{origin}
		bytes := 0

		skip := int64(query.Skip) + 1

		dir := int64(1)
		if query.Reverse {
			dir = int64(-1)
		}

		for len(headers) < int(query.Amount) && bytes < softResponseLimit && len(headers) < downloader.MaxHeaderFetch {
			block := origin.Number.Int64()
			block = block + (dir)*skip

			if block < 0 {
				break
			}
			origin, ok = e.blockchain.GetHeaderByNumber(big.NewInt(block))
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

		var hashes []common.Hash
		if err := msg.Decode(&hashes); err != nil {
			return err
		}

		// need to use the encoded version to keep track of the byte size
		bodies := []rlp.RawValue{}
		bytes := 0

		for i := 0; i < len(hashes) && bytes < softResponseLimit && len(bodies) < downloader.MaxBlockFetch; i++ {
			hash := hashes[i]

			body, ok := e.blockchain.GetBodyByHash(hash)
			if ok {
				data, err := rlp.EncodeToBytes(body)
				if err != nil {
					return err
				}

				bodies = append(bodies, data)
				bytes += len(data)
			}
		}
		return e.sendBlockBodies(bodies)

	case code == BlockBodiesMsg:
		var bodies BlockBodiesData
		if err := msg.Decode(&bodies); err != nil {
			return err
		}
		e.Bodies(bodies)

	case code == GetNodeDataMsg:
		// TODO. send

	case code == NodeDataMsg:
		var data [][]byte

		fmt.Println("-- size --")
		fmt.Println(msg.Size)

		if err := msg.Decode(&data); err != nil {
			panic(err)
		}
		e.Data(data)

	case code == GetReceiptsMsg:
		defer metrics.MeasureSince([]string{"minimal", "ethereum", "getReceipts"}, time.Now())

		var hashes []common.Hash
		if err := msg.Decode(&hashes); err != nil {
			return err
		}

		// need to use the encoded version to keep track of the byte size
		receipts := []rlp.RawValue{}
		bytes := 0

		for i := 0; i < len(hashes) && bytes < softResponseLimit && len(receipts) < downloader.MaxReceiptFetch; i++ {
			hash := hashes[i]

			res := e.blockchain.GetReceiptsByHash(hash)
			if res == nil {
				header, ok := e.blockchain.GetHeaderByHash(hash)
				if !ok || header.ReceiptHash != types.EmptyRootHash {
					continue
				}
			}

			data, err := rlp.EncodeToBytes(res)
			if err != nil {
				return err // log
			}
			receipts = append(receipts, data)
			bytes += len(data)
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

func (e *Ethereum) handleNewBlockHashesMsg(msg rlpx.Message) error {
	return nil
}

func (e *Ethereum) handleNewBlockMsg(msg rlpx.Message) error {
	var request newBlockData
	if err := msg.Decode(&request); err != nil {
		return err
	}

	a := request.Block.Number()
	b := request.TD.Int64()
	c := request.Block.Difficulty().Int64()

	// trueTD := new(big.Int).Sub(request.TD, request.Block.Difficulty())
	trueTD := request.TD

	fmt.Printf("===> NOTIFY Block: %d %d. Difficulty %d. Total: %d\n", a.Uint64(), c, trueTD.Uint64(), b)

	if trueTD.Cmp(e.HeaderDiff) > 0 {
		e.headerLock.Lock()
		e.HeaderDiff = request.TD
		e.HeaderHash = request.Block.Hash() // NOTE. not sure about this thing of not addedd yet
		e.headerLock.Unlock()

		go e.backend.notifyNewData(&NotifyMsg{Block: request.Block, Peer: e.peer, Diff: trueTD})
	} else {
		fmt.Println("-- NO UPDATE --")
		fmt.Println(trueTD.Int64())
		fmt.Println(e.HeaderDiff.Int64())
	}

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

// blockBody represents the data content of a single block.
type blockBody struct {
	Transactions []*types.Transaction // Transactions contained within a block
	Uncles       []*types.Header      // Uncles contained within a block
}

// BlockBodiesData is the network packet for block content distribution.
type BlockBodiesData []*blockBody

func (e *Ethereum) sendBlockBodies(bodies []rlp.RawValue) error {
	return e.writeMsg(BlockBodiesMsg, bodies)
}

func (e *Ethereum) sendReceipts(receipts []rlp.RawValue) error {
	return e.writeMsg(ReceiptsMsg, receipts)
}

// -- handlers --

// AckMessage is the ack message
type AckMessage struct {
	Complete bool
	Result   interface{}
}

type callback struct {
	ack chan AckMessage
}

// RequestHeaderByHashSync requests a header hash synchronously
func (e *Ethereum) RequestHeaderByHashSync(ctx context.Context, hash common.Hash) (*types.Header, error) {
	ack := make(chan AckMessage, 1)
	e.setHandler(ctx, headerMsg, hash.String(), ack)

	if err := e.RequestHeadersByHash(hash, 1, 0, false); err != nil {
		return nil, err
	}
	resp := <-ack
	if !resp.Complete {
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
	if !resp.Complete {
		return nil, false, fmt.Errorf("failed")
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

// RequestHeadersSync requests headers and waits for the response
func (e *Ethereum) RequestHeadersSync(ctx context.Context, origin uint64, count uint64) ([]*types.Header, error) {
	hash := strconv.Itoa(int(origin))

	ack := make(chan AckMessage, 1)
	e.setHandler(ctx, headerMsg, hash, ack)

	if err := e.RequestHeadersByNumber(origin, count, 0, false); err != nil {
		return nil, err
	}
	resp := <-ack
	if !resp.Complete {
		return nil, fmt.Errorf("failed")
	}

	response := resp.Result.([]*types.Header)
	return response, nil
}

// RequestReceiptsSync requests receipts and waits for the response
func (e *Ethereum) RequestReceiptsSync(ctx context.Context, hash string, hashes []common.Hash) ([][]*types.Receipt, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	ack := make(chan AckMessage, 1)
	e.setHandler(ctx, receiptsMsg, hash, ack)

	if err := e.RequestReceipts(hashes); err != nil {
		return nil, err
	}
	resp := <-ack
	if !resp.Complete {
		return nil, fmt.Errorf("failed")
	}

	// TODO. handle malformed response in the receipts
	response := resp.Result.([][]*types.Receipt)
	return response, nil
}

// RequestBodiesSync requests bodies and waits for the response
func (e *Ethereum) RequestBodiesSync(ctx context.Context, hash string, hashes []common.Hash) ([]*types.Body, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	ack := make(chan AckMessage, 1)
	e.setHandler(ctx, bodyMsg, hash, ack)

	if err := e.RequestBodies(hashes); err != nil {
		return nil, err
	}
	resp := <-ack
	if !resp.Complete {
		return nil, fmt.Errorf("failed")
	}

	// TODO. handle malformed response in the bodies
	response := resp.Result.(BlockBodiesData)

	res := []*types.Body{}
	for _, r := range response {
		res = append(res, &types.Body{Transactions: r.Transactions, Uncles: r.Uncles})
	}

	return res, nil
}

// RequestNodeDataSync requests node data and waits for the response
func (e *Ethereum) RequestNodeDataSync(ctx context.Context, hashes []common.Hash) ([][]byte, error) {
	ack := make(chan AckMessage, 1)
	e.setHandler(ctx, dataMsg, hashes[0].String(), ack)

	if err := e.RequestNodeData(hashes); err != nil {
		return nil, err
	}
	resp := <-ack
	if !resp.Complete {
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
		case ack <- AckMessage{false, nil}:
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
	case callback.ack <- AckMessage{Complete: true, Result: result}:
	default:
	}
	return true
}

// -- downloader --

// Headers receives the headers
func (e *Ethereum) Headers(headers []*types.Header) {
	if len(headers) != 0 {
		// request the header by number registers the number of the peer
		number := headers[0].Number.String()
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
		origin = types.DeriveSha(types.Receipts(receipts[0])).String()
	}
	if e.consumeHandler(origin, receiptsMsg, receipts) {
		return
	}
}

// Bodies receives the bodies
func (e *Ethereum) Bodies(bodies BlockBodiesData) {
	origin := ""
	if len(bodies) != 0 {
		first := bodies[0]
		origin = encodeHash(types.CalcUncleHash(first.Uncles), types.DeriveSha(types.Transactions(first.Transactions))).String()
	}
	if e.consumeHandler(origin, bodyMsg, bodies) {
		return
	}
}

// Data receives the node state data
func (e *Ethereum) Data(data [][]byte) {
	origin := ""
	if len(data) != 0 {
		origin = hexutil.Encode(crypto.Keccak256(data[0]))
	}
	if e.consumeHandler(origin, dataMsg, data) {
		return
	}
}

func encodeHash(x common.Hash, y common.Hash) common.Hash {
	hw := sha3.NewLegacyKeccak256()
	if _, err := hw.Write(x.Bytes()); err != nil {
		panic(err)
	}
	if _, err := hw.Write(y.Bytes()); err != nil {
		panic(err)
	}

	var h common.Hash
	hw.Sum(h[:0])
	return h
}
