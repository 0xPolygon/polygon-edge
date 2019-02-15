package network

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/armon/go-metrics"

	"github.com/ferranbt/periodic-dispatcher"

	"github.com/ethereum/go-ethereum/p2p/discv5"
	"github.com/umbracle/minimal/network/discovery"
	"github.com/umbracle/minimal/network/rlpx"
	"github.com/umbracle/minimal/protocol"
)

const (
	DialTimeout = 15 * time.Second
	peersFile   = "peers.json"
)

// Config is the p2p server configuration
type Config struct {
	Name              string
	BindAddress       string
	BindPort          int
	MaxPeers          int
	Bootnodes         []string
	DialTasks         int
	DialBusyInterval  time.Duration
	DiscoveryBackends map[string]discovery.Factory
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	c := &Config{
		Name:             "Minimal/go1.10.2",
		BindAddress:      "127.0.0.1",
		BindPort:         30304,
		MaxPeers:         10,
		Bootnodes:        []string{},
		DialTasks:        5,
		DialBusyInterval: 1 * time.Minute,
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

	info *rlpx.Info

	config  *Config
	closeCh chan struct{}

	Enode   string
	EventCh chan MemberEvent

	// set of pending nodes
	pendingNodes sync.Map

	addPeer chan string

	dispatcher *periodic.Dispatcher

	peerStore *PeerStore
	listener  *rlpx.Listener

	discovery discovery.Backend
}

// NewServer creates a new node
func NewServer(name string, key *ecdsa.PrivateKey, config *Config, logger *log.Logger) *Server {
	enode := fmt.Sprintf("enode://%s@%s:%d", discv5.PubkeyID(&key.PublicKey), config.BindAddress, config.BindPort)

	s := &Server{
		Protocols:    []*protocolStub{},
		Name:         name,
		key:          key,
		peers:        map[string]*Peer{},
		peersLock:    sync.Mutex{},
		config:       config,
		logger:       logger,
		closeCh:      make(chan struct{}),
		Enode:        enode,
		EventCh:      make(chan MemberEvent, 20),
		pendingNodes: sync.Map{},
		addPeer:      make(chan string, 20),
		dispatcher:   periodic.NewDispatcher(),
		peerStore:    NewPeerStore(peersFile),
	}

	return s
}

func (s *Server) buildInfo() {
	info := &rlpx.Info{
		Version: rlpx.BaseProtocolVersion,
		Caps:    rlpx.Capabilities{},
		Name:    s.Name,
		ID:      discv5.PubkeyID(&s.key.PublicKey),
	}
	for _, p := range s.Protocols {
		info.Caps = append(info.Caps, &rlpx.Cap{Name: p.protocol.Name, Version: p.protocol.Version})
	}
	s.info = info
}

// Schedule starts all the tasks once all the protocols have been loaded
func (s *Server) Schedule() error {
	// bootstrap peers
	for _, peer := range s.peerStore.Load() {
		s.Dial(peer)
	}

	// Create rlpx info
	s.buildInfo()

	if err := s.setupTransport(); err != nil {
		return err
	}

	// setup discovery factories
	discoveryConfig := &discovery.BackendConfig{
		Logger:  s.logger,
		Key:     s.key,
		Address: &net.TCPAddr{IP: net.ParseIP(s.config.BindAddress), Port: s.config.BindPort},
		Config:  map[string]interface{}{},
	}

	disc, ok := s.config.DiscoveryBackends["devp2p"]
	if ok {
		// Add the bootnodes. Maybe do this is a prestep in agent. If there are any bootnodes
		// in genesis file create a devp2p config with the list so that
		// server does not know about any bootnodes.
		discoveryConfig.Config["bootnodes"] = s.config.Bootnodes

		backend, err := disc(context.Background(), discoveryConfig)
		if err != nil {
			return err
		}

		s.discovery = backend
		s.discovery.Schedule()
	}

	go s.dialRunner()
	return nil
}

func (s *Server) setupTransport() error {
	addr := net.TCPAddr{IP: net.ParseIP(s.config.BindAddress), Port: s.config.BindPort}

	var err error
	s.listener, err = rlpx.Listen("tcp", addr.String(), &rlpx.Config{Prv: s.key, Info: s.info})
	if err != nil {
		return err
	}

	go func() {
		for {
			// TODO, Accept should check if we have enough slots or return tooManyPeers message
			conn, err := s.listener.Accept()
			if err != nil {
				// log
			}
			go s.handleIncomingConn(conn)
		}
	}()
	return nil
}

func (s *Server) handleIncomingConn(conn *rlpx.Session) {
	// check if we have enough incoming slots
	s.addSession(conn)
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
	s.logger.Printf("Dial task %s running", id)

	for {
		select {
		case task := <-tasks:
			s.logger.Printf("DIAL (%s): %s", id, task)

			err := s.connect(task)

			contains := s.dispatcher.Contains(task)
			busy := false
			if err != nil {
				if err == rlpx.DiscTooManyPeers {
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

		case enode := <-s.discovery.Deliver():
			fmt.Printf("FROM DISCOVER: %s\n", enode)
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
	for x, i := range s.peers {
		if id == x {
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
		p.Close()
	}
}

var (
	handlers = map[string]func(*Server, string) error{
		"enode": (*Server).connectWithEnode,
	}
)

func (s *Server) connect(addrs string) error {
	for n, h := range handlers {
		if strings.HasPrefix(addrs, n) {
			return h(s, addrs)
		}
	}
	return fmt.Errorf("Cannot connect to address %s", addrs)
}

func (s *Server) connectWithEnode(addrs string) error {
	ss, err := rlpx.DialEnode("tcp", addrs, &rlpx.Config{Prv: s.key, Info: s.info})
	if err != nil {
		return err
	}

	// match protocols
	return s.addSession(ss)
}

func (s *Server) addSession(session *rlpx.Session) error {
	p := newPeer(s.logger, session, session.RemoteInfo(), s)

	instances := s.matchProtocols(p, p.Info.Caps)
	if len(instances) == 0 {
		return fmt.Errorf("no matching protocols found")
	}

	for _, i := range instances {
		if err := i.Runtime.Init(); err != nil {
			return err
		}
	}
	p.SetInstances(instances)

	s.peersLock.Lock()
	s.peers[session.RemoteIDString()] = p
	s.peersLock.Unlock()

	select {
	case s.EventCh <- MemberEvent{Type: NodeJoin, Peer: p}:
	default:
	}

	return nil
}

// Callback is the one calling whenever the protocol is used
type Callback = func(session rlpx.Conn, peer *Peer) protocol.Handler

// RegisterProtocol registers a protocol
func (s *Server) RegisterProtocol(p protocol.Protocol, callback Callback) {
	s.Protocols = append(s.Protocols, &protocolStub{&p, callback})
}

func (s *Server) ID() discv5.NodeID {
	return discv5.PubkeyID(&s.key.PublicKey)
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
	offset := rlpx.BaseProtocolLength
	protocols := []*Instance{}

	for _, i := range caps {
		if proto := s.getProtocol(i.Name, i.Version); proto != nil {
			stream := peer.conn.(*rlpx.Session).OpenStream(uint(offset), uint(proto.protocol.Length))
			runtime := proto.callback(stream, peer)
			protocols = append(protocols, &Instance{session: stream, protocol: proto.protocol, offset: uint64(offset), Runtime: runtime})
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
		fmt.Println("-- update --")
		fmt.Println(i.Enode)

		s.peerStore.Update(i.Enode, i.Status)
	}
	if err := s.peerStore.Save(); err != nil {
		panic(err)
	}

	// close listener transport
	if err := s.listener.Close(); err != nil {
		// log
	}
}
