package network

import (
	"fmt"
	"log"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/umbracle/minimal/network/rlpx"
	"github.com/umbracle/minimal/protocol"
)

type Instance struct {
	session  *rlpx.Stream // session of the peer with the protocoll
	protocol *protocol.Protocol
	Runtime  protocol.Handler
	offset   uint64
}

type Status int

const (
	PeerActive Status = iota
	PeerPending
	PeerDisconnected
	PeerBusy
)

func (s Status) String() string {
	switch s {
	case PeerActive:
		return "active"

	case PeerPending:
		return "pending"

	case PeerDisconnected:
		return "disconnected"

	default:
		panic(fmt.Sprintf("Status %d not found", s))
	}
}

// Peer is each of the connected peers
type Peer struct {
	Enode     string
	ID        string
	Status    Status
	logger    *log.Logger
	conn      rlpx.Conn
	Info      *rlpx.Info
	protocols []*Instance

	headerHash common.Hash
	headerDiff *big.Int
	headerLock sync.Mutex
}

func newPeer(logger *log.Logger, conn rlpx.Conn, info *rlpx.Info, server *Server) *Peer {
	enode := fmt.Sprintf("enode://%s@%s", info.ID.String(), conn.RemoteAddr())

	peer := &Peer{
		Enode:      enode,
		ID:         info.ID.String(),
		logger:     logger,
		conn:       conn,
		Info:       info,
		protocols:  []*Instance{},
		headerLock: sync.Mutex{},
	}

	return peer
}

func (p *Peer) IsClosed() bool {
	return p.conn.(*rlpx.Session).IsClosed()
}

func (p *Peer) PrettyString() string {
	return p.ID[:8]
}

// UpdateHeader updates the header of the peer
func (p *Peer) UpdateHeader(h common.Hash, d *big.Int) {
	p.headerLock.Lock()
	p.headerHash = h
	p.headerDiff = d
	p.headerLock.Unlock()
}

// HeaderHash returns the header hash of the peer
func (p *Peer) HeaderHash() common.Hash {
	p.headerLock.Lock()
	defer p.headerLock.Unlock()
	return p.headerHash
}

// HeaderDiff returns the header difficulty of the peer
func (p *Peer) HeaderDiff() *big.Int {
	p.headerLock.Lock()
	defer p.headerLock.Unlock()
	return p.headerDiff
}

func (p *Peer) GetProtocol(name string, version uint) protocol.Handler {
	for _, i := range p.protocols {
		if i.protocol.Name == name && i.protocol.Version == version {
			return i.Runtime
		}
	}
	return nil
}

func (p *Peer) Close() {
	p.conn.Close()
}

func (p *Peer) GetProtocols() []*Instance {
	return p.protocols
}

func (p *Peer) SetInstances(protocols []*Instance) {
	p.protocols = protocols
}

func (p *Peer) getProtocol(msgcode uint64) *Instance {
	for _, proto := range p.protocols {
		if msgcode >= proto.offset && msgcode < proto.offset+proto.protocol.Length {
			return proto
		}
	}
	return nil
}
