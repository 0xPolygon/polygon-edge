package ethereum

import (
	"bytes"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/armon/go-metrics"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/rlp"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/umbracle/minimal/network"
	"github.com/umbracle/minimal/network/transport/rlpx"
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

// Downloader ingest the data
type Downloader interface {
	Headers([]*types.Header)
	Receipts([][]*types.Receipt)
	Bodies(BlockBodiesData)
	Data([][]byte)
}

// Blockchain is the interface the ethereum protocol needs to work
type Blockchain interface {
	GetHeaderByHash(hash common.Hash) *types.Header
	GetHeaderByNumber(n *big.Int) *types.Header
	GetReceiptsByHash(hash common.Hash) types.Receipts
	GetBodyByHash(hash common.Hash) *types.Body
}

// Ethereum is the protocol for etheruem
type Ethereum struct {
	conn     net.Conn
	sendLock sync.Mutex

	peer       *network.Peer
	getStatus  GetStatus
	status     *Status // status of the remote peer
	blockchain Blockchain
	downloader Downloader

	// pendin objects
	pending     map[string]*callback
	pendingLock sync.Mutex
	timer       *time.Timer

	header rlpx.Header
}

// GetStatus is the interface that gives the eth protocol the information it needs
type GetStatus func() (*Status, error)

// NewEthereumProtocol creates the ethereum protocol
func NewEthereumProtocol(conn net.Conn, peer *network.Peer, getStatus GetStatus, blockchain Blockchain) *Ethereum {
	return &Ethereum{
		conn:        conn,
		peer:        peer,
		getStatus:   getStatus,
		blockchain:  blockchain,
		pending:     make(map[string]*callback),
		pendingLock: sync.Mutex{},
		header:      make([]byte, rlpx.HeaderSize),
	}
}

// SetDownloader changes the downloader that ingests the data
func (e *Ethereum) SetDownloader(downloader Downloader) {
	e.downloader = downloader
}

func (e *Ethereum) Header() common.Hash {
	return e.peer.HeaderHash()
}

// Status is the object for the status message.
type Status struct {
	ProtocolVersion uint32
	NetworkID       uint64
	TD              *big.Int
	CurrentBlock    common.Hash
	GenesisBlock    common.Hash
}

// Requester is the etheruem protocol interface
type Requester interface {
	RequestHeadersByNumber(number uint64, amount uint64, skip uint64, reverse bool) error
	RequestHeadersByHash(origin common.Hash, amount uint64, skip uint64, reverse bool) error
	RequestBodies(hashes []common.Hash) error
	RequestNodeData(hashes []common.Hash) error
	RequestReceipts(hashes []common.Hash) error
}

// getBlockHeadersData represents a block header query.
type getBlockHeadersData struct {
	Origin  hashOrNumber
	Amount  uint64
	Skip    uint64
	Reverse bool
}

func (e *Ethereum) WriteMsg(code int, data interface{}) error {
	e.sendLock.Lock()
	defer e.sendLock.Unlock()

	r, err := rlp.EncodeToBytes(data)
	if err != nil {
		return err
	}

	e.header.Encode(uint16(code), uint32(len(r)))

	if _, err := e.conn.Write(e.header[:]); err != nil {
		return err
	}
	if _, err := e.conn.Write(r); err != nil {
		return err
	}
	return nil
}

// RequestHeadersByNumber fetches a batch of blocks' headers based on the number of an origin block.
func (e *Ethereum) RequestHeadersByNumber(number uint64, amount uint64, skip uint64, reverse bool) error {
	return e.WriteMsg(GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Number: number}, Amount: amount, Skip: skip, Reverse: reverse})
}

// RequestHeadersByHash fetches a batch of blocks' headers based on the hash of an origin block.
func (e *Ethereum) RequestHeadersByHash(origin common.Hash, amount uint64, skip uint64, reverse bool) error {
	return e.WriteMsg(GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Hash: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse})
}

// RequestBodies fetches a batch of blocks' bodies based on a set of hashes.
func (e *Ethereum) RequestBodies(hashes []common.Hash) error {
	return e.WriteMsg(GetBlockBodiesMsg, hashes)
}

// RequestNodeData fetches a batch of arbitrary data from a node's known state.
func (e *Ethereum) RequestNodeData(hashes []common.Hash) error {
	return e.WriteMsg(GetNodeDataMsg, hashes)
}

// RequestReceipts fetches a batch of transaction receipts from a remote node.
func (e *Ethereum) RequestReceipts(hashes []common.Hash) error {
	return e.WriteMsg(GetReceiptsMsg, hashes)
}

// Conn returns the connection referece
func (e *Ethereum) Conn() *rlpx.Stream {
	return nil
	// return e.conn.(*rlpx.Stream)
}

func (e *Ethereum) ReadStatus() (*Status, error) {
	var status *Status
	if _, err := e.conn.Read(e.header[:]); err != nil {
		return nil, err
	}
	if code := e.header.MsgType(); code != StatusMsg {
		return nil, fmt.Errorf("Message code is not statusMsg but %d", code)
	}
	buf := make([]byte, e.header.Length())
	if _, err := e.conn.Read(buf); err != nil {
		return nil, err
	}
	if err := rlp.DecodeBytes(buf, &status); err != nil {
		return nil, err
	}
	return status, nil
}

func (e *Ethereum) ValidateStatus(remoteStatus *Status, localStatus *Status) error {
	if remoteStatus.NetworkID != localStatus.NetworkID {
		return fmt.Errorf("Network id does not match. Found %d but expected %d", remoteStatus.NetworkID, localStatus.NetworkID)
	}
	if remoteStatus.GenesisBlock != localStatus.GenesisBlock {
		return fmt.Errorf("Genesis block does not match")
	}
	if int(remoteStatus.ProtocolVersion) != int(localStatus.ProtocolVersion) {
		return fmt.Errorf("Protocol version does not match. Found %d but expected %d", int(remoteStatus.ProtocolVersion), int(localStatus.ProtocolVersion))
	}
	return nil
}

func (e *Ethereum) ReadAndValidateStatus(localStatus *Status) error {
	var err error
	e.status, err = e.ReadStatus()
	if err != nil {
		return err
	}
	return e.ValidateStatus(e.status, localStatus)
}

// Close the protocol
func (e *Ethereum) Close() error {
	return nil
}

func (e *Ethereum) Status() *Status {
	return e.status
}

// Init starts the protocol
func (e *Ethereum) Init() error {
	status, err := e.getStatus()
	if err != nil {
		return err
	}

	errr := make(chan error, 2)

	go func() {
		errr <- e.ReadAndValidateStatus(status)
	}()

	go func() {
		errr <- e.WriteMsg(StatusMsg, status)
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

	// e.peer.UpdateHeader(e.status.CurrentBlock, e.status.TD)

	// handshake was correct, start to listen for packets
	go e.listen()
	return nil
}

func (e *Ethereum) listen() {
	for {
		if _, err := e.conn.Read(e.header[:]); err != nil {
			panic(err)
		}

		buf := make([]byte, e.header.Length())
		if _, err := e.conn.Read(buf); err != nil {
			panic(err)
		}

		msg := rlpx.Message{
			Code:    uint64(e.header.MsgType()),
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
	metrics.IncrCounterWithLabels([]string{"minimal", "ethereum", "msg"}, float32(1.0), []metrics.Label{{Name: "Code", Value: strconv.Itoa(int(msg.Code))}})

	code := msg.Code

	switch {
	case code == StatusMsg:
		return fmt.Errorf("Status msg not expected after handshake")

	case code == GetBlockHeadersMsg:
		defer metrics.MeasureSince([]string{"minimal", "ethereum", "getHeaders"}, time.Now())

		var query getBlockHeadersData
		err := msg.Decode(&query)
		if err != nil {
			return err
		}

		var origin *types.Header
		if query.Origin.IsHash() {
			origin = e.blockchain.GetHeaderByHash(query.Origin.Hash)
		} else {
			origin = e.blockchain.GetHeaderByNumber(big.NewInt(int64(query.Origin.Number)))
		}

		if origin == nil {
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
			origin = e.blockchain.GetHeaderByNumber(big.NewInt(block))
			if origin == nil {
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
		defer metrics.MeasureSince([]string{"minimal", "ethereum", "getBodies"}, time.Now())

		var hashes []common.Hash
		if err := msg.Decode(&hashes); err != nil {
			return err
		}

		// need to use the encoded version to keep track of the byte size
		bodies := []rlp.RawValue{}
		bytes := 0

		for i := 0; i < len(hashes) && bytes < softResponseLimit && len(bodies) < downloader.MaxBlockFetch; i++ {
			hash := hashes[i]

			body := e.blockchain.GetBodyByHash(hash)
			if body != nil {
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
				header := e.blockchain.GetHeaderByHash(hash)
				if header == nil || header.ReceiptHash != types.EmptyRootHash {
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
		// TODO. notify announce

	case code == NewBlockMsg:
		var request newBlockData
		if err := msg.Decode(&request); err != nil {
			return err
		}

		trueHead := request.Block.ParentHash()
		trueTD := new(big.Int).Sub(request.TD, request.Block.Difficulty())

		if td := e.peer.HeaderDiff(); trueTD.Cmp(td) > 0 {
			e.peer.UpdateHeader(trueHead, trueTD)
		}
		// TODO: notify the syncer about the new block (syncer interface as in blockchain?)

	case code == TxMsg:
		// TODO: deliver

	default:
		return fmt.Errorf("Message code %d not found", code)
	}

	return nil
}

// sendBlockHeaders sends a batch of block headers to the remote peer.
func (e *Ethereum) sendBlockHeaders(headers []*types.Header) error {
	return e.WriteMsg(BlockHeadersMsg, headers)
}

// blockBody represents the data content of a single block.
type blockBody struct {
	Transactions []*types.Transaction // Transactions contained within a block
	Uncles       []*types.Header      // Uncles contained within a block
}

// BlockBodiesData is the network packet for block content distribution.
type BlockBodiesData []*blockBody

func (e *Ethereum) sendBlockBodies(bodies []rlp.RawValue) error {
	return e.WriteMsg(BlockBodiesMsg, bodies)
}

func (e *Ethereum) sendReceipts(receipts []rlp.RawValue) error {
	return e.WriteMsg(ReceiptsMsg, receipts)
}

// -- handlers --

// AckMessage is the ack message
type AckMessage struct {
	Complete bool
	Result   interface{}
}

type callback struct {
	id  uint32
	ack chan AckMessage
}

// RequestHeadersSync requests headers and waits for the response
func (e *Ethereum) RequestHeadersSync(origin uint64, count uint64) ([]*types.Header, error) {
	hash := strconv.Itoa(int(origin))

	ack := make(chan AckMessage, 1)
	e.setHandler(hash, 1, ack)

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
func (e *Ethereum) RequestReceiptsSync(hash string, hashes []common.Hash) ([][]*types.Receipt, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	/*
		hashes := []common.Hash{}
		for _, b := range receipts {
			hashes = append(hashes, b.Hash())
		}

		hash := receipts[0].ReceiptHash.String()
	*/

	ack := make(chan AckMessage, 1)
	e.setHandler(hash, 1, ack)

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
func (e *Ethereum) RequestBodiesSync(hash string, hashes []common.Hash) ([]*types.Body, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	/*
		hashes := []common.Hash{}
		for _, b := range bodies {
			hashes = append(hashes, b.Hash())
		}

		first := bodies[0]
		hash := encodeHash(first.UncleHash, first.TxHash).String()
	*/

	ack := make(chan AckMessage, 1)
	e.setHandler(hash, 1, ack)

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
func (e *Ethereum) RequestNodeDataSync(hashes []common.Hash) ([][]byte, error) {
	ack := make(chan AckMessage, 1)
	e.setHandler(hashes[0].String(), 1, ack)

	if err := e.RequestNodeData(hashes); err != nil {
		return nil, err
	}
	resp := <-ack
	if !resp.Complete {
		return nil, fmt.Errorf("failed")
	}

	return resp.Result.([][]byte), nil
}

func (e *Ethereum) setHandler(key string, id uint32, ack chan AckMessage) error {
	e.pendingLock.Lock()
	e.pending[key] = &callback{id, ack}
	e.pendingLock.Unlock()

	e.timer = time.AfterFunc(5*time.Second, func() {
		e.pendingLock.Lock()
		if _, ok := e.pending[key]; !ok {
			e.pendingLock.Unlock()
			return
		}

		delete(e.pending, key)
		e.pendingLock.Unlock()

		select {
		case ack <- AckMessage{false, nil}:
		default:
		}
	})

	return nil
}

func (e *Ethereum) consumeHandler(origin string, result interface{}) bool {
	e.pendingLock.Lock()
	callback, ok := e.pending[origin]
	if !ok {
		e.pendingLock.Unlock()
		return false
	}

	// delete
	delete(e.pending, origin)
	e.pendingLock.Unlock()

	// let him know its over
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
		hash := headers[0].Number.String()
		if e.consumeHandler(hash, headers) {
			return
		}
	}
	if e.downloader != nil {
		e.downloader.Headers(headers)
	}
}

// Receipts receives the receipts
func (e *Ethereum) Receipts(receipts [][]*types.Receipt) {
	if len(receipts) != 0 {
		hash := types.DeriveSha(types.Receipts(receipts[0]))
		if e.consumeHandler(hash.String(), receipts) {
			return
		}
	}
	if e.downloader != nil {
		e.downloader.Receipts(receipts)
	}
}

// Bodies receives the bodies
func (e *Ethereum) Bodies(bodies BlockBodiesData) {
	if len(bodies) != 0 {
		first := bodies[0]
		hash := encodeHash(types.CalcUncleHash(first.Uncles), types.DeriveSha(types.Transactions(first.Transactions)))
		if e.consumeHandler(hash.String(), bodies) {
			return
		}
	}
	if e.downloader != nil {
		e.downloader.Bodies(bodies)
	}
}

// Data receives the node state data
func (e *Ethereum) Data(data [][]byte) {
	if len(data) != 0 {
		hash := hexutil.Encode(crypto.Keccak256(data[0]))
		if e.consumeHandler(hash, data) {
			return
		}
	}
	if e.downloader != nil {
		e.downloader.Data(data)
	}
}

func encodeHash(x common.Hash, y common.Hash) common.Hash {
	hw := sha3.NewKeccak256()
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
