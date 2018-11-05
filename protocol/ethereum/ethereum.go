package ethereum

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/umbracle/minimal/network"
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

// Ethereum is the protocol for etheruem
type Ethereum struct {
	conn      network.Conn
	peer      *network.Peer
	getStatus GetStatus
	status    *Status
}

// GetStatus is the interface that gives the eth protocol the information it needs
type GetStatus func() (*Status, error)

// NewEthereumProtocol creates the ethereum protocol
func NewEthereumProtocol(conn network.Conn, peer *network.Peer, getStatus GetStatus) *Ethereum {
	return &Ethereum{
		conn:      conn,
		peer:      peer,
		getStatus: getStatus,
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
	RequestHeadersByHash(origin common.Hash, amount int, skip int, reverse bool) error
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
func (e *Ethereum) RequestHeadersByHash(origin common.Hash, amount int, skip int, reverse bool) error {
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

func (e *Ethereum) readStatus(localStatus *Status) (*Status, error) {
	msg, err := e.conn.ReadMsg()
	if err != nil {
		return nil, err
	}
	if msg.Code != StatusMsg {
		return nil, fmt.Errorf("")
	}

	var status Status
	if err := rlp.DecodeBytes(msg.Payload, &status); err != nil {
		return nil, err
	}

	if status.GenesisBlock != localStatus.GenesisBlock {
		return nil, fmt.Errorf("")
	}
	if status.NetworkID != localStatus.NetworkID {
		return nil, fmt.Errorf("")
	}
	if int(status.ProtocolVersion) != int(localStatus.ProtocolVersion) {
		return nil, fmt.Errorf("")
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
		if err := <-errr; err != nil {
			return err
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
	switch {
	case code == StatusMsg:
		return fmt.Errorf("Status msg not expected after handshake")

	case code == GetBlockHeadersMsg:
		// TODO. send

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
