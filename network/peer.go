package network

import (
	"fmt"
	"log"

	"github.com/umbracle/minimal/network/common"
)

/*
type Instance struct {
	Protocol protocol.Protocol
	Handler  protocol.Handler
}
*/

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
	Enode  string
	ID     string
	Status Status
	logger *log.Logger
	//conn      *rlpx.Session
	conn2 common.Session
	//Info      *rlpx.Info // remove info and conn
	protocols []*common.Instance
}

func newPeer2(logger *log.Logger, conn2 common.Session, server *Server) *Peer {
	enode := conn2.GetInfo().Enode.String()

	peer := &Peer{
		Enode:     enode,
		ID:        conn2.GetInfo().Enode.ID.String(),
		logger:    logger,
		conn2:     conn2,
		protocols: []*common.Instance{},
	}

	return peer
}

/*
func newPeer(logger *log.Logger, conn *rlpx.Session, info *rlpx.Info, server *Server) *Peer {
	enode := fmt.Sprintf("enode://%s@%s", info.ID.String(), conn.RemoteAddr())

	peer := &Peer{
		Enode:  enode,
		ID:     info.ID.String(),
		logger: logger,
		conn:   conn,
		Info:   info,
		// protocols: map[string]*Instance{},
	}

	return peer
}
*/

/*
func (p *Peer) IsClosed() bool {
	return p.conn.(*rlpx.Session).IsClosed()
}
*/

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

// Close closes the peer connection
func (p *Peer) Close() {
	p.conn2.Close()
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
