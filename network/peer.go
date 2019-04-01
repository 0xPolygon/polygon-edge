package network

import (
	"fmt"
	"log"

	"github.com/umbracle/minimal/network/common"
)

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
	conn      common.Session
	protocols []*common.Instance
}

func newPeer(logger *log.Logger, conn common.Session, server *Server) *Peer {
	enode := conn.GetInfo().Enode.String()

	peer := &Peer{
		Enode:     enode,
		ID:        conn.GetInfo().Enode.ID.String(),
		logger:    logger,
		conn:      conn,
		protocols: []*common.Instance{},
	}

	return peer
}

func (p *Peer) IsClosed() bool {
	return p.conn.IsClosed()
}

func (p *Peer) PrettyString() string {
	return p.ID[:8]
}

// Close closes the peer connection
func (p *Peer) Close() {
	p.conn.Close()
}
