package network

import (
	"crypto/ecdsa"
	"fmt"
	"log"
	"net"
	"sort"
	"time"

	"github.com/umbracle/minimal/network/discover"
	"github.com/umbracle/minimal/protocol"
)

const (
	DialTimeout = 5 * time.Second
)

// Config is the p2p server configuration
type Config struct {
	Name        string
	BindAddress string
	BindPort    int
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	c := &Config{
		Name:        "Minimal/go1.10.2",
		BindAddress: "0.0.0.0",
		BindPort:    30304,
	}

	return c
}

type protocolStub struct {
	protocol *protocol.Protocol
	callback Callback
}

type EventType int

const (
	NodeJoin EventType = iota
	NodeLeave
	NodeHandshakeFail
)

func (t EventType) String() string {
	switch t {
	case NodeJoin:
		return "node join"
	case NodeLeave:
		return "node leave"
	case NodeHandshakeFail:
		return "node handshake failed"
	default:
		panic(fmt.Sprintf("unknown event type: %d", t))
	}
}

type MemberEvent struct {
	Type EventType
	Peer *Peer
}

// Server is the ethereum client
type Server struct {
	logger     *log.Logger
	Protocols  []*protocolStub
	Name       string
	key        *ecdsa.PrivateKey
	peers      []*Peer
	info       *Info
	config     *Config
	shutdownCh chan struct{}
	transport  Transport
	routing    *discover.RoutingTable
	Enode      string
	EventCh    chan MemberEvent
}

// NewServer creates a new node
func NewServer(name string, key *ecdsa.PrivateKey, config *Config, logger *log.Logger) (*Server, error) {

	// Start transport
	nc := &NetTransportConfig{
		BindAddrs: []string{config.BindAddress},
		BindPort:  config.BindPort,
		Logger:    logger,
	}

	transport, err := NewNetTransport(nc)
	if err != nil {
		return nil, err
	}

	enode := fmt.Sprintf("enode://%s@%s:%d", discover.PubkeyToNodeID(&key.PublicKey), config.BindAddress, config.BindPort)

	s := &Server{
		Protocols:  []*protocolStub{},
		Name:       name,
		key:        key,
		peers:      []*Peer{},
		config:     config,
		logger:     logger,
		shutdownCh: make(chan struct{}),
		transport:  transport,
		Enode:      enode,
		EventCh:    make(chan MemberEvent, 20),
	}

	udpAddr := &net.UDPAddr{IP: net.ParseIP(config.BindAddress), Port: config.BindPort}

	s.routing, err = discover.NewRoutingTable(key, transport, udpAddr, discover.DefaultConfig())
	if err != nil {
		return nil, err
	}

	s.routing.Schedule()
	go s.streamListen()

	return s, nil
}

func (s *Server) streamListen() {
	for {
		select {
		case conn := <-s.transport.StreamCh():
			go s.handleIncommingConn(conn)
		case <-s.shutdownCh:
			return
		}
	}
}

func (s *Server) handleIncommingConn(conn net.Conn) {
	secrets, err := doEncHandshake(conn, s.key, nil)
	if err != nil {
		s.logger.Printf("failed to handle handshake: %v", err)
		return
	}

	upgraded, err := newConnection(conn, secrets)
	if err != nil {
		s.logger.Printf("%v", err)
		return
	}

	s.HandleConn(upgraded)
}

func (s *Server) GetPeer(id string) *Peer {
	for _, i := range s.peers {
		if i.Info.ID.String() == id {
			return i
		}
	}
	return nil
}

func (s *Server) Disconnect() {
	// disconnect the peers
	for _, p := range s.peers {
		p.Disconnect(DiscRequested)
	}
}

// Dial calls a specific node
func (s *Server) Dial(nodeStr string) error {
	node, err := discover.ParseNode(nodeStr)
	if err != nil {
		return err
	}

	addr := &net.TCPAddr{IP: node.IP, Port: int(node.TCP)}
	conn, err := s.transport.DialTimeout(addr.String(), DialTimeout)
	if err != nil {
		return err
	}

	rpub, err := node.ID.Pubkey()
	if err != nil {
		panic(err)
	}

	secrets, err := doEncHandshake(conn, s.key, rpub)
	if err != nil {
		return fmt.Errorf("handshake failed: %v", err)
	}

	upgraded, err := newConnection(conn, secrets)
	if err != nil {
		return fmt.Errorf("connection failed: %v", err)
	}

	go s.HandleConn(upgraded)
	return nil
}

// Event manager to let them know when there is a new peer

// HandleConn it is called ansycronous
func (s *Server) HandleConn(conn *Connection) {
	nodeInfo := s.getServerCapabilities()

	other, err := startProtocolHandshake(conn, nodeInfo)
	if err != nil {
		panic(err)
	}

	// sorted capabilities
	sort.Sort(other.Caps)

	peer := newPeer(s.logger, conn, other)
	peer.SetInstances(s.matchProtocols(peer, other.Caps))
	s.peers = append(s.peers, peer)

	go peer.listen()

	// run the handshake protocols
	for _, i := range peer.protocols {
		if err := i.Runtime.Init(); err != nil {
			peer.Close()

			s.EventCh <- MemberEvent{NodeHandshakeFail, peer}
			return
		}
	}

	s.EventCh <- MemberEvent{NodeJoin, peer}
}

// Callback is the one calling whenever the protocol is used
type Callback = func(session Conn, peer *Peer) protocol.Handler

// RegisterProtocol registers a protocol
func (s *Server) RegisterProtocol(p protocol.Protocol, callback Callback) {
	s.Protocols = append(s.Protocols, &protocolStub{&p, callback})
}

func (s *Server) ID() discover.NodeID {
	return discover.PubkeyToNodeID(&s.key.PublicKey)
}

func (s *Server) getServerCapabilities() *Info {
	ourHandshake := &Info{Version: baseProtocolVersion, Caps: Capabilities{}, Name: s.Name, ID: discover.PubkeyToNodeID(&s.key.PublicKey)}
	for _, p := range s.Protocols {
		ourHandshake.Caps = append(ourHandshake.Caps, &Cap{Name: p.protocol.Name, Version: p.protocol.Version})
	}
	return ourHandshake
}

func (s *Server) getProtocol(name string, version uint) *protocolStub {
	for _, p := range s.Protocols {
		if p.protocol.Name == name && p.protocol.Version == version {
			return p
		}
	}
	return nil
}

func (s *Server) matchProtocols(peer *Peer, caps Capabilities) []*Instance {
	offset := baseProtocolLength
	protocols := []*Instance{}

	for _, i := range caps {
		if proto := s.getProtocol(i.Name, i.Version); proto != nil {
			session := NewSession(offset, peer.conn)
			runtime := proto.callback(session, peer)

			protocols = append(protocols, &Instance{session: session, protocol: proto.protocol, offset: offset, Runtime: runtime})
			offset += proto.protocol.Length
		}
	}

	return protocols
}
