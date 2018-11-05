package parity

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/umbracle/minimal/network"
	"github.com/umbracle/minimal/protocol/ethereum"

	"github.com/ethereum/go-ethereum/common"
)

// parity protocol message codes
const (
	// Protocol messages belonging to par/1
	SnapshotManifestPacket    = 0x11
	GetSnapshotManifestPacket = 0x12
	SnapshotDataPacket        = 0x13
	GetSnapshotDataPacket     = 0x14
)

// Parity is the protocol for etheruem
type Parity struct {
	conn      network.Conn
	peer      *network.Peer
	ethereum  *ethereum.Ethereum
	getStatus GetStatus
	status    *Status // status of the remote peer
}

// GetStatus is the interface that gives the eth protocol the information it needs
type GetStatus func() (*Status, error)

// NewParityProtocol creates the parity protocol
func NewParityProtocol(conn network.Conn, peer *network.Peer, getStatus GetStatus) *Parity {
	return &Parity{
		conn:      conn,
		peer:      peer,
		ethereum:  ethereum.NewEthereumProtocol(conn, peer, nil),
		getStatus: getStatus,
	}
}

// Close closes the protocol connection
func (p *Parity) Close() error {
	return nil
}

// Status is the status message from parity (includes manifesthash and blocknumber)
type Status struct {
	ProtocolVersion uint8
	NetworkID       uint64
	TD              *big.Int
	CurrentBlock    common.Hash
	GenesisBlock    common.Hash
	SnapshotHash    common.Hash
	SnapshotNumber  *big.Int
}

// Requester is the parity requester actions
type Requester interface {
	ethereum.Requester
	GetSnapshotManifestPacket() error
	GetSnapshotDataPacket(hash common.Hash) error
}

// -- parity specific requesters --

// RequestSnapshotManifestPacket fetches the snapshot manifest
func (p *Parity) RequestSnapshotManifestPacket() error {
	return p.conn.WriteMsg(SnapshotManifestPacket, struct{}{})
}

// RequestSnapshotDataPacket fetches a snapshot packet based on the packet hash
func (p *Parity) RequestSnapshotDataPacket(hash common.Hash) error {
	return p.conn.WriteMsg(SnapshotDataPacket, []common.Hash{hash})
}

// -- ethereum protocol requesters --

// RequestHeadersByNumber fetches a batch of blocks' headers based on the number of an origin block.
func (p *Parity) RequestHeadersByNumber(number uint64, amount uint64, skip uint64, reverse bool) error {
	return p.ethereum.RequestHeadersByNumber(number, amount, skip, reverse)
}

// RequestHeadersByHash fetches a batch of blocks' headers based on the hash of an origin block.
func (p *Parity) RequestHeadersByHash(origin common.Hash, amount int, skip int, reverse bool) error {
	return p.ethereum.RequestHeadersByHash(origin, amount, skip, reverse)
}

// RequestBodies fetches a batch of blocks' bodies based on a set of hashes.
func (p *Parity) RequestBodies(hashes []common.Hash) error {
	return p.ethereum.RequestBodies(hashes)
}

// RequestNodeData fetches a batch of arbitrary data from a node's known state.
func (p *Parity) RequestNodeData(hashes []common.Hash) error {
	return p.ethereum.RequestNodeData(hashes)
}

// RequestReceipts fetches a batch of transaction receipts from a remote node.
func (p *Parity) RequestReceipts(hashes []common.Hash) error {
	return p.ethereum.RequestReceipts(hashes)
}

func (p *Parity) readStatus(localStatus *Status) (*Status, error) {
	msg, err := p.conn.ReadMsg()
	if err != nil {
		return nil, err
	}
	if msg.Code != ethereum.StatusMsg {
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

// Init starts the protocol
func (p *Parity) Init() error {
	status, err := p.getStatus()
	if err != nil {
		return err
	}

	var peerStatus *Status
	errr := make(chan error, 2)

	go func() {
		peerStatus, err = p.readStatus(status)
		errr <- err
	}()

	go func() {
		errr <- p.conn.WriteMsg(ethereum.StatusMsg, status)
	}()

	for i := 0; i < 2; i++ {
		if err := <-errr; err != nil {
			return err
		}
	}

	p.status = peerStatus

	// handshake was correct, start to listen for packets
	go p.listen()

	return nil
}

func (p *Parity) listen() {
	for {
		msg, err := p.conn.ReadMsg()
		if err != nil {
			panic(err)
		}

		if err := p.HandleMsg(msg.Code, msg.Payload); err != nil {
			// close connection
			p.conn.Close()
		}
	}
}

// HandleMsg handles a message from ethereum
func (p *Parity) HandleMsg(code uint64, payload []byte) error {
	switch code {
	case SnapshotManifestPacket:
		// TODO. deliver

	case GetSnapshotManifestPacket:
		// TODO. send

	case SnapshotDataPacket:
		// TODO. deliver

	case GetSnapshotDataPacket:
		// TODO. send

	default:
		return p.ethereum.HandleMsg(code, payload)
	}

	return nil
}
