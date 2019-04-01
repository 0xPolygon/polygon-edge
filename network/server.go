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

	"github.com/umbracle/minimal/network/transport/rlpx"

	"github.com/umbracle/minimal/helper/enode"

	"github.com/armon/go-metrics"

	"github.com/ferranbt/periodic-dispatcher"

	"github.com/umbracle/minimal/network/common"
	"github.com/umbracle/minimal/network/discovery"
	"github.com/umbracle/minimal/protocol"
)

const (
	peersFile = "peers.json"
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
	ServiceName       string
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
		ServiceName:      "minimal",
	}

	return c
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
	logger *log.Logger
	// Protocols []*protocolStub
	Name string
	key  *ecdsa.PrivateKey

	peersLock sync.Mutex
	peers     map[string]*Peer

	info *common.Info

	config  *Config
	closeCh chan struct{}
	EventCh chan MemberEvent

	// set of pending nodes
	pendingNodes sync.Map

	addPeer chan string

	dispatcher *periodic.Dispatcher

	peerStore *PeerStore
	listener  net.Listener
	transport common.Transport

	discovery discovery.Backend
	Enode     *enode.Enode

	backends []protocol.Backend
}

// NewServer creates a new node
func NewServer(name string, key *ecdsa.PrivateKey, config *Config, logger *log.Logger) *Server {
	enode := &enode.Enode{
		IP:  net.ParseIP(config.BindAddress),
		TCP: uint16(config.BindPort),
		UDP: uint16(config.BindPort),
		ID:  enode.PubkeyToEnode(&key.PublicKey),
	}

	fmt.Printf("Enode: %s\n", enode.String())

	s := &Server{
		// Protocols:    []*protocolStub{},
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
		backends:     []protocol.Backend{},
		transport:    &rlpx.Rlpx{},
	}

	return s
}

func (s *Server) buildInfo() {
	info := &common.Info{
		Client: s.Name,
		Enode:  s.Enode,
	}

	for _, p := range s.backends {
		proto := p.Protocol()

		cap := &common.Capability{
			Protocol: proto,
			Backend:  p,
		}

		info.Capabilities = append(info.Capabilities, cap)
	}
	s.info = info
}

/*
func (s *Server) buildInfo2() {
	info := &rlpx.Info{
		Version: rlpx.BaseProtocolVersion,
		Caps:    rlpx.Capabilities{},
		Name:    s.Name,
		ID:      enode.PubkeyToEnode(&s.key.PublicKey),
	}

	fmt.Println("-- build info --")
	fmt.Println("-- backends --")
	fmt.Println(s.backends)

	for _, p := range s.backends {
		proto := p.Protocol()
		info.Caps = append(info.Caps, &rlpx.Cap{Name: proto.Name, Version: proto.Version})
	}
	s.info = info
}
*/

// Schedule starts all the tasks once all the protocols have been loaded
func (s *Server) Schedule() error {
	// bootstrap peers
	for _, peer := range s.peerStore.Load() {
		s.Dial(peer)
	}

	// Create rlpx info
	s.buildInfo()

	s.transport.Setup(s.key, s.backends, s.info)

	if err := s.setupTransport(); err != nil {
		return err
	}

	// setup discovery factories
	discoveryConfig := &discovery.BackendConfig{
		Logger: s.logger,
		Key:    s.key,
		Enode:  s.Enode,
		Config: map[string]interface{}{},
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

	discoveryConfig.Config["nodename"] = s.config.ServiceName

	for name, disc := range s.config.DiscoveryBackends {
		fmt.Printf("Using discovery %s\n", name)
		backend, err := disc(context.Background(), discoveryConfig)
		if err != nil {
			return err
		}
		s.discovery = backend // NOTE: Find a way to tunnel allt he discovery results
		backend.Schedule()
	}

	go s.dialRunner()
	return nil
}

/*
func (s *Server) setupTransport2() error {
	l, err := net.Listen("tcp", "")
	if err != nil {
		return err
	}

	s.lis = l
	go s.startAccepting()

	return nil
}

func (s *Server) startAccepting() {
	for {
		conn, err := s.lis.Accept()
		if err != nil {
			s.logger.Printf("[ERR]: %v", err)
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	session, err := s.transport.Accept(conn)
	if err != nil {
		s.logger.Printf("[ERR]: %v", err)
	}

	s.addSession(session)
}
*/

func (s *Server) setupTransport() error {

	addr := net.TCPAddr{IP: net.ParseIP(s.config.BindAddress), Port: s.config.BindPort}

	var err error
	s.listener, err = net.Listen("tcp", addr.String())
	if err != nil {
		return err
	}

	go func() {
		for {
			conn, err := s.listener.Accept()
			if err != nil {
				// log
			}
			go s.handleConn(conn)
		}
	}()

	/*
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
	*/

	return nil
}

func (s *Server) handleConn(conn net.Conn) {
	session, err := s.transport.Accept(conn)
	if err != nil {
		panic(err)
	}

	if err := s.addSession(session); err != nil {
		panic(err)
	}
}

/*
func (s *Server) handleIncomingConn(conn *rlpx.Session) {
	// check if we have enough incoming slots

	fmt.Println("-- conn --")
	fmt.Println(conn)

	s.addSession(conn)
}
*/

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
				fmt.Printf("ERR: %v\n", err)

				/*
					if err == rlpx.DiscTooManyPeers {
						busy = true
					}
					// TODO
				*/
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

			/*
				case enode := <-s.discovery.Deliver():
					// fmt.Printf("FROM DISCOVER: %s\n", enode)
					sendToTask(enode)
			*/

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

// TODO, build a method that takes a net.Conn object
// it will be helpful to tests protocols. connectWithEnode
// would call this function after all the connection has been established

func (s *Server) connectWithEnode(rawURL string) error {
	// parse enode address beforehand
	// TODO, make dial take either a rawURL or an enode
	addr, err := enode.ParseURL(rawURL)
	if err != nil {
		return err
	}

	if _, ok := s.peers[addr.ID.String()]; ok {
		// TODO: add tests
		// Trying to connect with an already connected id
		// TODO, after disconnect do we remove the peer from this list?
		return nil
	}

	tcpAddr := addr.TCPAddr()
	conn, err := net.Dial("tcp", tcpAddr.String())
	if err != nil {
		return err
	}

	session, err := s.transport.Connect(conn, *addr)
	if err != nil {
		return err
	}

	/*
		ss, err := rlpx.DialEnode("tcp", rawURL, &rlpx.Config{Prv: s.key, Info: s.info})
		if err != nil {
			return err
		}
	*/

	// match protocols
	return s.addSession(session)
}

/*
func (s *Server) connectWithEnode2(rawURL string) error {
	// parse enode address beforehand
	// TODO, make dial take either a rawURL or an enode
	enode, err := enode.ParseURL(rawURL)
	if err != nil {
		return err
	}

	if _, ok := s.peers[enode.ID.String()]; ok {
		// TODO: add tests
		// Trying to connect with an already connected id
		// TODO, after disconnect do we remove the peer from this list?
		return nil
	}

	conn, err := net.Dial("tcp", enode.IP.String())
	if err != nil {
		return err
	}

	session, err := s.transport.Connect(conn, *enode)
	if err != nil {
		return err
	}

	// match protocols
	return s.addSession2(session)
}

func (s *Server) addSession2(session Session) error {
	p := newPeer2(s.logger, session, s)

	// should match protocols be part of rlpx?
	backends := s.matchProtocols(p, p.Info.Caps)
	if len(backends) == 0 {
		return fmt.Errorf("no matching protocols found")
	}

	// There should be a better way to handle the initialization of the peer
	// this should be done inside the peer itself not in the server.
	for _, i := range backends {
		if err := i.backend.Add(i.session, p.PrettyString()); err != nil {
			return err
		}
	}
	// p.SetInstances(instances)

	s.peersLock.Lock()
	s.peers[p.PrettyString()] = p
	s.peersLock.Unlock()

	select {
	case s.EventCh <- MemberEvent{Type: NodeJoin, Peer: p}:
	default:
	}

	return nil
}
*/

func (s *Server) addSession(session common.Session) error {
	// p := newPeer(s.logger, session, session.RemoteInfo(), s)
	p := newPeer2(s.logger, session, s)

	/*
		// should match protocols be part of rlpx?
		backends := s.matchProtocols(p, p.Info.Caps)
		if len(backends) == 0 {
			return fmt.Errorf("no matching protocols found")
		}

		// There should be a better way to handle the initialization of the peer
		// this should be done inside the peer itself not in the server.
		for _, i := range backends {
			if err := i.backend.Add(i.session, p.PrettyString()); err != nil {
				return err
			}
		}
	*/

	// TODO, during match protocols we need to return which backend protocols have been matched
	protos, err := session.NegociateProtocols(s.info)
	if err != nil {
		// send close message to the peer
		return err
	}

	// p.SetInstances(instances)

	p.protocols = protos

	s.peersLock.Lock()
	// s.peers[session.RemoteIDString()] = p
	s.peers[p.PrettyString()] = p
	s.peersLock.Unlock()

	select {
	case s.EventCh <- MemberEvent{Type: NodeJoin, Peer: p}:
	default:
	}

	return nil
}

// RegisterProtocol registers a protocol
func (s *Server) RegisterProtocol(b protocol.Backend) error {
	s.backends = append(s.backends, b)
	// TODO, check if the backend is already registered
	return nil
}

func (s *Server) ID() enode.ID {
	return s.Enode.ID
}

func (s *Server) getProtocol(name string, version uint) protocol.Backend {
	for _, p := range s.backends {
		proto := p.Protocol()
		if proto.Name == name && proto.Version == version {
			return p
		}
	}
	return nil
}

/*
func (s *Server) matchProtocols2(peer *Peer, caps rlpx.Capabilities) ([]*Instance, error) {
	offset := rlpx.BaseProtocolLength
	// protocols := []*Instance{}

	type res struct { // will become matchProtocol struct in rlpx
		offset   uint64
		protocol protocol.Protocol
		backend  protocol.Backend
	}

	result := []*res{}

	for _, i := range caps {
		if b := s.getProtocol(i.Name, i.Version); b != nil {
			proto := b.Protocol()

			result = append(result, &res{
				backend:  b,
				protocol: proto,
				offset:   offset,
			})

			offset += proto.Length
		}
	}

	lock := sync.Mutex{}
	activated := []*Instance{}

	errr := make(chan error, len(result))
	for _, r := range result {
		go func(r *res) {
			stream := peer.conn.OpenStream(uint(r.offset), uint(r.protocol.Length))

			proto, err := r.backend.Add(stream, peer.ID)
			if err != nil {
				errr <- err
			}

			lock.Lock()
			activated = append(activated, &Instance{
				protocol: r.protocol,
				handler:  proto,
			})
			lock.Unlock()
			errr <- nil
		}(r)
	}

	for i := 0; i < len(result); i++ {
		if err := <-errr; err != nil {
			return nil, err
		}
	}
	return activated, nil
}
*/

/*
func (s *Server) matchProtocols(peer *Peer, caps rlpx.Capabilities) []*Instance {
	offset := rlpx.BaseProtocolLength
	protocols := []*Instance{}

	for _, i := range caps {
		if b := s.getProtocol(i.Name, i.Version); b != nil {
			proto := b.Protocol()

			stream := peer.conn.OpenStream(uint(offset), uint(proto.Length))
			// runtime := proto.callback(stream, peer)

			instance := &Instance{
				session: stream,
				backend: b,
				offset:  offset,
			}

			protocols = append(protocols, instance)
			offset += proto.Length
		}
	}

	return protocols
}
*/

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
