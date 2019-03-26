package network

import (
	"fmt"
	"log"
	"net"

	"github.com/umbracle/minimal/network/transport/rlpx"
	"github.com/umbracle/minimal/protocol"
)

type Instance struct {
	session net.Conn // session of the peer with the protocoll
	// protocol *protocol.Protocol
	backend protocol.Backend
	// Runtime  protocol.Handler
	offset uint64
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
	Info      *rlpx.Info // remove info and conn
	protocols map[string]*Instance
}

func newPeer(logger *log.Logger, conn rlpx.Conn, info *rlpx.Info, server *Server) *Peer {
	enode := fmt.Sprintf("enode://%s@%s", info.ID.String(), conn.RemoteAddr())

	peer := &Peer{
		Enode:     enode,
		ID:        info.ID.String(),
		logger:    logger,
		conn:      conn,
		Info:      info,
		protocols: map[string]*Instance{},
	}

	return peer
}

func (p *Peer) IsClosed() bool {
	return p.conn.(*rlpx.Session).IsClosed()
}

func (p *Peer) PrettyString() string {
	return p.ID[:8]
}

/*
func (p *Peer) GetProtocol(name string) protocol.Handler {
	proto, ok := p.protocols[name]
	if !ok {
		return nil
	}
	return proto.Runtime
}
*/

func (p *Peer) Close() {
	p.conn.Close()
}

/*
func (p *Peer) SetInstances(protocols []*Instance) {
	for _, i := range protocols {
		p.protocols[i.protocol.Name] = i
	}
}

func (p *Peer) getProtocol(msgcode uint64) *Instance {
	for _, proto := range p.protocols {
		if msgcode >= proto.offset && msgcode < proto.offset+proto.protocol.Length {
			return proto
		}
	}
	return nil
}
*/
