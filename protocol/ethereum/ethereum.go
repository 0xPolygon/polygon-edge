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

// Blockchain is the interface the ethereum protocol needs to work
type Blockchain interface {
	GetHeaderByHash(hash common.Hash) *types.Header
	GetHeaderByNumber(n *big.Int) *types.Header
}

// Ethereum is the protocol for etheruem
type Ethereum struct {
	conn       network.Conn
	peer       *network.Peer
	getStatus  GetStatus
	status     *Status
	blockchain Blockchain
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

// HandleMsg handles a message from ethereum
func (e *Ethereum) HandleMsg(code uint64, payload []byte) error {
	if code != TxMsg && code != 0x1 {
		fmt.Printf("Code: %d\n", code)
	}

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
		bytes := common.StorageSize(estHeaderRlpSize)

		skip := int64(query.Skip)

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
		// TODO. deliver

	case code == GetBlockBodiesMsg:
		// TODO. send

	case code == BlockBodiesMsg:
		// TODO. deliver

	case code == GetNodeDataMsg:
		// TODO. send

	case code == NodeDataMsg:
		// TODO. deliver

	case code == GetReceiptsMsg:
		// TODO. send

	case code == ReceiptsMsg:
		// TODO. deliver

	case code == NewBlockHashesMsg:
		// TODO. notify announce

	case code == NewBlockMsg:
		// TODO: propagated block

	case code == TxMsg:
		// TODO: deliver

	default:
		return fmt.Errorf("Message code %d not found", code)
	}

	return nil
}

// SendBlockHeaders sends a batch of block headers to the remote peer.
func (e *Ethereum) sendBlockHeaders(headers []*types.Header) error {
	return e.conn.WriteMsg(BlockHeadersMsg, headers)
}
