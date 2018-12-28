package network

import (
	"crypto/ecdsa"
	"fmt"
	"log"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/armon/go-metrics"

	"github.com/ferranbt/periodic-dispatcher"

	"github.com/umbracle/minimal/network/discover"
	"github.com/umbracle/minimal/network/rlpx"
	"github.com/umbracle/minimal/protocol"
)

const (
	DialTimeout = 15 * time.Second
	peersFile   = "peers.json"
)

// Config is the p2p server configuration
type Config struct {
	Name             string
	BindAddress      string
	BindPort         int
	MaxPeers         int
	Bootnodes        []string
	DialTasks        int
	DialBusyInterval time.Duration
	Discover         *discover.Config
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	c := &Config{
		Name:             "Minimal/go1.10.2",
		BindAddress:      "0.0.0.0",
		BindPort:         30304,
		MaxPeers:         10,
		Bootnodes:        []string{},
		DialTasks:        5,
		DialBusyInterval: 1 * time.Minute,
		Discover:         discover.DefaultConfig(),
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
	logger    *log.Logger
	Protocols []*protocolStub
	Name      string
	key       *ecdsa.PrivateKey

	peersLock sync.Mutex
	peers     map[string]*Peer

	info      *rlpx.Info
	config    *Config
	closeCh   chan struct{}
	transport Transport
	discover  *discover.Discover
	Enode     string
	EventCh   chan MemberEvent

	// set of pending nodes
	pendingNodes sync.Map

	addPeer chan string

	dispatcher *periodic.Dispatcher

	peerStore *PeerStore
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
		Protocols:    []*protocolStub{},
		Name:         name,
		key:          key,
		peers:        map[string]*Peer{},
		peersLock:    sync.Mutex{},
		config:       config,
		logger:       logger,
		closeCh:      make(chan struct{}),
		transport:    transport,
		Enode:        enode,
		EventCh:      make(chan MemberEvent, 20),
		pendingNodes: sync.Map{},
		addPeer:      make(chan string, 20),
		dispatcher:   periodic.NewDispatcher(),
		peerStore:    NewPeerStore(peersFile),
	}

	udpAddr := &net.UDPAddr{IP: net.ParseIP(config.BindAddress), Port: config.BindPort}

	s.discover, err = discover.NewDiscover(key, transport, udpAddr, config.Discover)
	if err != nil {
		return nil, err
	}

	s.discover.SetBootnodes(config.Bootnodes)
	s.discover.Schedule()

	go s.streamListen()

	// bootstrap peers
	for _, peer := range s.peerStore.Load() {
		s.Dial(peer)
	}

	go s.dialRunner()

	return s, nil
}

func (s *Server) streamListen() {
	for {
		select {
		case conn := <-s.transport.StreamCh():
			go s.handleIncomingConn(conn)
		case <-s.closeCh:
			return
		}
	}
}

func (s *Server) handleIncomingConn(conn net.Conn) {
	if err := s.connect2(conn, nil); err != nil {
		s.logger.Printf("ERR: incoming connection %v", err)
	}
}

// PeriodicDial is the periodic dial of busy peers
type PeriodicDial struct {
	enode string
}

// ID returns the id of the enode
func (p *PeriodicDial) ID() string {
	return p.enode
}

// -- DIALING --

func (s *Server) dialTask(id string, tasks chan string) {
	for {
		select {
		case task := <-tasks:
			s.logger.Printf("DIAL (%s): %s", id, task)

			err := s.connectWithEnode(task)

			contains := s.dispatcher.Contains(task)
			busy := false
			if err != nil {
				if _, ok := err.(*rlpx.DiscMsgTooManyPeers); ok {
					busy = true
				}
			}

			if busy {
				// the peer had too many peers, reschedule to dial it again if it is not already on the list
				if !contains {
					if err := s.dispatcher.Add(&PeriodicDial{task}, s.config.DialBusyInterval); err != nil {
						// log
					}
				}
			} else {
				// either worked or failed for a reason different than 'too many peers'
				if contains {
					if err := s.dispatcher.Remove(task); err != nil {
						// log
					}
				}
			}

			metrics.IncrCounter([]string{"server", "dial task"}, 1.0)
		case <-s.closeCh:
			return
		}
	}
}

func (s *Server) dialRunner() {
	s.dispatcher.SetEnabled(true)

	tasks := make(chan string, s.config.DialTasks)

	// run the dialtasks
	for i := 0; i < s.config.DialTasks; i++ {
		go s.dialTask(strconv.Itoa(i), tasks)
	}

	sendToTask := func(enode string) {
		tasks <- enode
	}

	for {
		select {
		case enode := <-s.addPeer:
			sendToTask(enode)

		case enode := <-s.discover.EventCh:
			sendToTask(enode)

		case enode := <-s.dispatcher.Events():
			sendToTask(enode.ID())

		case <-s.closeCh:
			return
		}
	}
}

// Dial dials an enode (async)
func (s *Server) Dial(enode string) {
	select {
	case s.addPeer <- enode:
	default:
	}
}

// DialSync dials and waits for the result
func (s *Server) DialSync(enode string) error {
	return s.connectWithEnode(enode)
}

func (s *Server) GetPeer(id string) *Peer {
	for _, i := range s.peers {
		if i.Info.ID.String() == id {
			return i
		}
	}
	return nil
}

func (s *Server) removePeer(peer *Peer) {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	if _, ok := s.peers[peer.ID]; ok {
		delete(s.peers, peer.ID)
	}

	metrics.SetGauge([]string{"minimal", "peers"}, float32(len(s.peers)))
}

func (s *Server) Disconnect() {
	// disconnect the peers
	for _, p := range s.peers {
		p.Disconnect(DiscRequested)
	}
}

// MismatchProtocolError happens when the other peer has a different networkid or genesisblock
type MismatchProtocolError struct {
	Msg error
}

func (m *MismatchProtocolError) Error() string {
	return m.Msg.Error()
}

func (s *Server) connectWithEnode(enode string) error {
	node, err := discover.ParseNode(enode)
	if err != nil {
		return err
	}

	rpub, err := node.ID.Pubkey()
	if err != nil {
		return err
	}

	id := node.ID.String()

	// check if its already a peer
	s.peersLock.Lock()
	_, ok := s.peers[id]
	s.peersLock.Unlock()
	if ok {
		return nil
	}

	// check if its pending
	if _, ok := s.pendingNodes.Load(id); ok {
		return nil
	}

	// set as pending
	s.pendingNodes.Store(id, true)
	defer s.pendingNodes.Delete(id)

	addr := &net.TCPAddr{IP: node.IP, Port: int(node.TCP)}

	conn, err := s.transport.DialTimeout(addr.String(), DialTimeout)
	if err != nil {
		return err
	}

	if err := s.connect2(conn, rpub); err != nil {
		conn.Close()
		return err
	}
	return nil
}

func (s *Server) connect2(conn net.Conn, pub *ecdsa.PublicKey) error {
	peer, err := s.connect(conn, pub)
	if err != nil && peer == nil {
		s.EventCh <- MemberEvent{NodeHandshakeFail, nil}
		return err
	}

	id := peer.ID
	if err != nil {
		if peer != nil {
			peer.Close()
		}

		s.EventCh <- MemberEvent{NodeHandshakeFail, peer}
		return err
	} else {
		s.peersLock.Lock()
		s.peers[id] = peer
		metrics.SetGauge([]string{"minimal", "peers"}, float32(len(s.peers)))
		s.peersLock.Unlock()

		s.EventCh <- MemberEvent{NodeJoin, peer}
	}

	return nil
}

// do all the handshakes
func (s *Server) connect(conn net.Conn, pub *ecdsa.PublicKey) (*Peer, error) {

	// -- network handshake --
	secrets, err := rlpx.DoEncHandshake(conn, s.key, pub)
	if err != nil {
		return nil, fmt.Errorf("handshake failed: %v", err)
	}

	upgraded, err := rlpx.NewConnection(conn, secrets) // do this maybe on doEncHandhsake at the same time
	if err != nil {
		return nil, fmt.Errorf("connection failed: %v", err)
	}

	// -- protocol handshake --

	localInfo := s.getServerCapabilities()

	remoteInfo, err := rlpx.StartProtocolHandshake(upgraded, localInfo)
	if err != nil {
		return nil, err
	}
	sort.Sort(remoteInfo.Caps)

	peer := newPeer(s.logger, upgraded, remoteInfo, s)

	instances := s.matchProtocols(peer, remoteInfo.Caps)
	if len(instances) == 0 {
		return nil, fmt.Errorf("no matching protocols")
	}

	peer.SetInstances(instances)
	go peer.listen()

	// --- initialize the protocols

	errr := make(chan error, len(peer.protocols))

	for _, i := range peer.protocols {
		go func(i *Instance) {
			errr <- i.Runtime.Init()
		}(i)
	}

	for i := 0; i < len(peer.protocols); i++ {
		if err := <-errr; err != nil {
			return nil, err
		}
	}

	return peer, nil
}

// Callback is the one calling whenever the protocol is used
type Callback = func(session rlpx.Conn, peer *Peer) protocol.Handler

// RegisterProtocol registers a protocol
func (s *Server) RegisterProtocol(p protocol.Protocol, callback Callback) {
	s.Protocols = append(s.Protocols, &protocolStub{&p, callback})
}

func (s *Server) ID() discover.NodeID {
	return discover.PubkeyToNodeID(&s.key.PublicKey)
}

func (s *Server) getServerCapabilities() *rlpx.Info {
	ourHandshake := &rlpx.Info{Version: baseProtocolVersion, Caps: rlpx.Capabilities{}, Name: s.Name, ID: discover.PubkeyToNodeID(&s.key.PublicKey)}
	for _, p := range s.Protocols {
		ourHandshake.Caps = append(ourHandshake.Caps, &rlpx.Cap{Name: p.protocol.Name, Version: p.protocol.Version})
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

func (s *Server) matchProtocols(peer *Peer, caps rlpx.Capabilities) []*Instance {
	offset := baseProtocolLength
	protocols := []*Instance{}

	for _, i := range caps {
		if proto := s.getProtocol(i.Name, i.Version); proto != nil {
			session := rlpx.NewSession(offset, peer.conn)
			runtime := proto.callback(session, peer)

			protocols = append(protocols, &Instance{session: session, protocol: proto.protocol, offset: offset, Runtime: runtime})
			offset += proto.protocol.Length
		}
	}

	return protocols
}

func (s *Server) Close() {
	// close peers
	for _, i := range s.peers {
		i.Close()
	}

	for _, i := range s.peers {
		s.peerStore.Update(i.Enode, i.Status)
	}

	if err := s.peerStore.Save(); err != nil {
		panic(err)
	}
}
