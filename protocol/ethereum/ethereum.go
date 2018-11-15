package ethereum

import (
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/umbracle/minimal/network"
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
	conn       network.Conn
	peer       *network.Peer
	getStatus  GetStatus
	status     *Status
	blockchain Blockchain
	downloader Downloader
}

// GetStatus is the interface that gives the eth protocol the information it needs
type GetStatus func() (*Status, error)

// NewEthereumProtocol creates the ethereum protocol
func NewEthereumProtocol(conn network.Conn, peer *network.Peer, getStatus GetStatus, blockchain Blockchain) *Ethereum {
	return &Ethereum{
		conn:       conn,
		peer:       peer,
		getStatus:  getStatus,
		blockchain: blockchain,
	}
}

// SetDownloader changes the downloader that ingests the data
func (e *Ethereum) SetDownloader(downloader Downloader) {
	e.downloader = downloader
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

// RequestHeadersByNumber fetches a batch of blocks' headers based on the number of an origin block.
func (e *Ethereum) RequestHeadersByNumber(number uint64, amount uint64, skip uint64, reverse bool) error {
	return e.conn.WriteMsg(GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Number: number}, Amount: amount, Skip: skip, Reverse: reverse})
}

// RequestHeadersByHash fetches a batch of blocks' headers based on the hash of an origin block.
func (e *Ethereum) RequestHeadersByHash(origin common.Hash, amount uint64, skip uint64, reverse bool) error {
	return e.conn.WriteMsg(GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Hash: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse})
}

// RequestBodies fetches a batch of blocks' bodies based on a set of hashes.
func (e *Ethereum) RequestBodies(hashes []common.Hash) error {
	return e.conn.WriteMsg(GetBlockBodiesMsg, hashes)
}

// RequestNodeData fetches a batch of arbitrary data from a node's known state.
func (e *Ethereum) RequestNodeData(hashes []common.Hash) error {
	return e.conn.WriteMsg(GetNodeDataMsg, hashes)
}

// RequestReceipts fetches a batch of transaction receipts from a remote node.
func (e *Ethereum) RequestReceipts(hashes []common.Hash) error {
	return e.conn.WriteMsg(GetReceiptsMsg, hashes)
}

// Conn returns the connection referece
func (e *Ethereum) Conn() network.Conn {
	return e.conn
}

func (e *Ethereum) readStatus(localStatus *Status) (*Status, error) {
	msg, err := e.conn.ReadMsg()
	if err != nil {
		return nil, err
	}
	if msg.Code != StatusMsg {
		return nil, fmt.Errorf("Message code is not statusMsg but %d", msg.Code)
	}

	var status Status
	if err := rlp.DecodeBytes(msg.Payload, &status); err != nil {
		return nil, err
	}

	if status.NetworkID != localStatus.NetworkID {
		return nil, &network.MismatchProtocolError{Msg: fmt.Errorf("Network id does not match. Found %d but expected %d", status.NetworkID, localStatus.NetworkID)}
	}
	if status.GenesisBlock != localStatus.GenesisBlock {
		return nil, &network.MismatchProtocolError{Msg: fmt.Errorf("Genesis block does not match")}
	}
	if int(status.ProtocolVersion) != int(localStatus.ProtocolVersion) {
		return nil, fmt.Errorf("Protocol version does not match. Found %d but expected %d", int(status.ProtocolVersion), int(localStatus.ProtocolVersion))
	}

	return &status, nil
}

// Close the protocol
func (e *Ethereum) Close() error {
	return nil
}

// Init starts the protocol
func (e *Ethereum) Init() error {
	status, err := e.getStatus()
	if err != nil {
		return err
	}

	var peerStatus *Status
	errr := make(chan error, 2)

	go func() {
		peerStatus, err = e.readStatus(status)
		errr <- err
	}()

	go func() {
		errr <- e.conn.WriteMsg(StatusMsg, status)
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

	e.status = peerStatus
	e.peer.UpdateHeader(peerStatus.CurrentBlock, peerStatus.TD)

	// handshake was correct, start to listen for packets
	go e.listen()
	return nil
}

func (e *Ethereum) listen() {
	for {
		msg, err := e.conn.ReadMsg()
		if err != nil {
			panic(err)
		}

		if err := e.HandleMsg(msg.Code, msg.Payload); err != nil {
			// close connection
			e.conn.Close()
		}
	}
}

type newBlockData struct {
	Block *types.Block
	TD    *big.Int
}

// HandleMsg handles a message from ethereum
func (e *Ethereum) HandleMsg(code uint64, payload []byte) error {
	switch {
	case code == StatusMsg:
		return fmt.Errorf("Status msg not expected after handshake")

	case code == GetBlockHeadersMsg:
		var query getBlockHeadersData
		err := rlp.DecodeBytes(payload, &query)
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
		if err := rlp.DecodeBytes(payload, &headers); err != nil {
			return err
		}

		if e.downloader != nil {
			e.downloader.Headers(headers)
		}
	case code == GetBlockBodiesMsg:
		var hashes []common.Hash
		if err := rlp.DecodeBytes(payload, &hashes); err != nil {
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
		var request BlockBodiesData
		if err := rlp.DecodeBytes(payload, &request); err != nil {
			return err
		}

		if e.downloader != nil {
			e.downloader.Bodies(request)
		}
	case code == GetNodeDataMsg:
		// TODO. send

	case code == NodeDataMsg:
		// TODO. deliver

	case code == GetReceiptsMsg:
		var hashes []common.Hash
		if err := rlp.DecodeBytes(payload, &hashes); err != nil {
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
		if err := rlp.DecodeBytes(payload, &receipts); err != nil {
			return err
		}

		if e.downloader != nil {
			e.downloader.Receipts(receipts)
		}
	case code == NewBlockHashesMsg:
		// TODO. notify announce

	case code == NewBlockMsg:
		var request newBlockData
		if err := rlp.DecodeBytes(payload, &request); err != nil {
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
	return e.conn.WriteMsg(BlockHeadersMsg, headers)
}

// blockBody represents the data content of a single block.
type blockBody struct {
	Transactions []*types.Transaction // Transactions contained within a block
	Uncles       []*types.Header      // Uncles contained within a block
}

// BlockBodiesData is the network packet for block content distribution.
type BlockBodiesData []*blockBody

func (e *Ethereum) sendBlockBodies(bodies []rlp.RawValue) error {
	return e.conn.WriteMsg(BlockBodiesMsg, bodies)
}

func (e *Ethereum) sendReceipts(receipts []rlp.RawValue) error {
	return e.conn.WriteMsg(ReceiptsMsg, receipts)
}
