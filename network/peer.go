package network

import (
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/umbracle/minimal/network/discover"
	"github.com/umbracle/minimal/protocol"
)

const (
	baseProtocolVersion    = 5
	baseProtocolLength     = uint64(16)
	baseProtocolMaxMsgSize = 2 * 1024

	snappyProtocolVersion = 5

	defaultPingInterval = 15 * time.Second
)

type DiscReason uint

const (
	DiscRequested DiscReason = iota
	DiscNetworkError
	DiscProtocolError
	DiscUselessPeer
	DiscTooManyPeers
	DiscAlreadyConnected
	DiscIncompatibleVersion
	DiscInvalidIdentity
	DiscQuitting
	DiscUnexpectedIdentity
	DiscSelf
	DiscReadTimeout
	DiscSubprotocolError = 0x10
)

func (d DiscReason) String() string {
	switch d {
	case DiscRequested:
		return "disconnect requested"
	case DiscNetworkError:
		return "network error"
	case DiscProtocolError:
		return "breach of protocol"
	case DiscUselessPeer:
		return "useless peer"
	case DiscTooManyPeers:
		return "too many peers"
	case DiscAlreadyConnected:
		return "already connected"
	case DiscIncompatibleVersion:
		return "incompatible p2p protocol version"
	case DiscInvalidIdentity:
		return "invalid node identity"
	case DiscQuitting:
		return "client quitting"
	case DiscUnexpectedIdentity:
		return "unexpected identity"
	case DiscSelf:
		return "connected to self"
	case DiscReadTimeout:
		return "read timeout"
	case DiscSubprotocolError:
		return "subprotocol error"
	default:
		panic(fmt.Errorf("Disc reason %d not found", d))
	}
}

func (d DiscReason) Error() string {
	return d.String()
}

func decodeDiscMsg(msg []byte) (DiscReason, error) {
	var reason [1]DiscReason
	if err := rlp.DecodeBytes(msg, &reason); err != nil {
		return 0x0, err
	}
	return reason[0], nil
}

// Cap is the peer capability.
type Cap struct {
	Name    string
	Version uint
}

func (c *Cap) less(cc *Cap) bool {
	if cmp := strings.Compare(c.Name, cc.Name); cmp != 0 {
		return cmp == -1
	}
	return c.Version < cc.Version
}

// Capabilities are all the capabilities of the other peer
type Capabilities []*Cap

func (c Capabilities) Len() int {
	return len(c)
}

func (c Capabilities) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c Capabilities) Less(i, j int) bool {
	return c[i].less(c[j])
}

// Info is the info of the node
type Info struct {
	Version    uint64
	Name       string
	Caps       Capabilities
	ListenPort uint64
	ID         discover.NodeID

	// Ignore additional fields (for forward compatibility).
	Rest []rlp.RawValue `rlp:"tail"`
}

const (
	// devp2p message codes
	handshakeMsg = 0x00
	discMsg      = 0x01
	pingMsg      = 0x02
	pongMsg      = 0x03
)

type Instance struct {
	session  *Session // session of the peer with the protocoll
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
	}
	panic(fmt.Sprintf("Status %d not found", s))
}

// Peer is each of the connected peers
type Peer struct {
	Enode        string
	ID           string
	Status       Status
	logger       *log.Logger
	conn         Conn
	Info         *Info
	protocols    []*Instance
	Connected    bool
	pongTimeout  *time.Timer
	closeCh      chan struct{}
	pingInterval time.Duration

	headerHash common.Hash
	headerDiff *big.Int
	headerLock sync.Mutex
}

func newPeer(logger *log.Logger, conn *Connection, info *Info) *Peer {
	enode := fmt.Sprintf("enode://%s@%s", info.ID.String(), conn.conn.RemoteAddr().String())

	peer := &Peer{
		Enode:        enode,
		ID:           info.ID.String(),
		logger:       logger,
		conn:         conn,
		Info:         info,
		Connected:    true,
		pongTimeout:  time.NewTimer(10 * time.Second),
		closeCh:      make(chan struct{}),
		pingInterval: defaultPingInterval,
		protocols:    []*Instance{},
		headerLock:   sync.Mutex{},
	}

	return peer
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

func (p *Peer) Schedule() {
	go p.startPing()
	go p.listen()
}

func (p *Peer) startPing() {
	// not sure if send an initial ping here
	for {
		select {
		case <-time.After(p.pingInterval):
			go p.Ping()
		case <-p.pongTimeout.C:
			p.Close()
			return
		case <-p.closeCh:
			return
		}
	}
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
	// Close protocols
	/*
		for _, i := range p.protocols {
			if err := i.Runtime.Close(); err != nil {
				p.logger.Printf("failed to close protocol %s: %v\n", i.protocol.Name, err)
			}
		}
	*/

	p.conn.Close()
	close(p.closeCh)
	p.Connected = false
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

func (p *Peer) handleMsg(msg Message) error {
	p.pongTimeout.Reset(2 * time.Second)

	switch {
	case msg.Code == pingMsg:
		p.Pong()
	case msg.Code == pongMsg:
		// already handled
	case msg.Code == discMsg: // handleDiscMessage
		reason, err := decodeDiscMsg(msg.Payload)
		if err != nil {
			p.logger.Printf("failed to decode disc msg: %v\n", err)
		} else {
			p.logger.Printf("Disconnected: %s\n", reason)
		}
		p.Connected = false
	default:
		pp := p.getProtocol(msg.Code)
		if pp == nil {
			return fmt.Errorf("Protocol for msg %d not found", msg.Code)
		}

		real := msg.Copy()
		real.Code = real.Code - pp.offset

		if !pp.session.Consume(real.Code, real.Payload) {
			pp.session.msgs <- *real
		}
	}

	return nil
}

func (p *Peer) listen() {
	for {
		msg, err := p.conn.ReadMsg()
		if err != nil {
			p.Connected = false
			return
		}

		if err := p.handleMsg(msg); err != nil {
			p.logger.Printf("handle msg err: %v\n", err)
		}
	}
}

// Ping the peer
func (p *Peer) Ping() {
	if err := p.conn.WriteMsg(pingMsg); err != nil {
		p.logger.Printf("failed to send ping message: %v\n", err)
	}
}

// Pong the peer
func (p *Peer) Pong() {
	if err := p.conn.WriteMsg(pongMsg); err != nil {
		p.logger.Printf("failed to send pong message: %v\n", err)
	}
}

// Disconnect sends a disconnect message with a reason
func (p *Peer) Disconnect(reason DiscReason) {
	if err := p.conn.WriteMsg(discMsg, []DiscReason{reason}); err != nil {
		p.logger.Printf("failed to send disc message: %v\n", err)
	}
}
