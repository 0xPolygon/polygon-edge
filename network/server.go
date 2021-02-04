package network

import (
	"crypto/ecdsa"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/minimal/helper/enode"

	"github.com/armon/go-metrics"

	"github.com/ferranbt/periodic-dispatcher"

	"github.com/0xPolygon/minimal/network/discovery"
)

// Protocol is a wire protocol
type Protocol struct {
	Spec      ProtocolSpec
	HandlerFn func(conn net.Conn, peer *Peer) (ProtocolHandler, error)
}

// ProtocolHandler is the handler of the protocol
type ProtocolHandler interface {
	Info() (map[string]interface{}, error)
}

// ProtocolSpec is a specification of an etheruem protocol
type ProtocolSpec struct {
	Name    string
	Version uint
	Length  uint64
}

// Info is the information of a peer
type Info struct {
	Client       string
	Enode        *enode.Enode
	Capabilities Capabilities
	ListenPort   uint64
}

// Capability is a feature of the peer
type Capability struct {
	Protocol Protocol
}

// Capabilities is a list of capabilities of the peer
type Capabilities []*Capability

type Instance struct {
	Protocol *Protocol
	Handler  ProtocolHandler
}

const (
	defaultDialTimeout = 10 * time.Second
	defaultDialTasks   = 15
)

// Config is the p2p server configuration
type Config struct {
	Name             string
	DataDir          string
	BindAddress      string
	BindPort         int
	MaxPeers         int
	Bootnodes        []string
	DialTasks        int
	DialBusyInterval time.Duration
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	c := &Config{
		Name:             "Minimal/go1.10.2",
		BindAddress:      "127.0.0.1",
		BindPort:         30304,
		MaxPeers:         10,
		Bootnodes:        []string{},
		DialTasks:        defaultDialTasks,
		DialBusyInterval: 1 * time.Minute,
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
	logger hclog.Logger
	Name   string
	key    *ecdsa.PrivateKey

	consensus consensus.Consensus

	peersLock sync.Mutex
	peers     map[string]*Peer

	info *Info

	config  *Config
	closeCh chan struct{}
	EventCh chan MemberEvent

	// set of pending nodes
	pendingNodes sync.Map

	addPeer chan string

	dispatcher *periodic.Dispatcher

	peerStore PeerStore
	transport Transport

	Discovery discovery.Backend
	Enode     *enode.Enode

	backends []*Protocol
}

// NewServer creates a new node
func NewServer(name string, key *ecdsa.PrivateKey, config *Config, logger hclog.Logger, transport Transport) *Server {
	enode := &enode.Enode{
		IP:  net.ParseIP(config.BindAddress),
		TCP: uint16(config.BindPort),
		UDP: uint16(config.BindPort),
		ID:  enode.PubkeyToEnode(&key.PublicKey),
	}

	logger.Info("ID", "enode", enode.String())

	s := &Server{
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
		peerStore:    &NoopPeerStore{},
		backends:     []*Protocol{},
		transport:    transport,
	}

	return s
}

// SetPeerStore sets the peerstore
func (s *Server) SetPeerStore(p PeerStore) {
	s.peerStore = p
}

// SetConsensus sets the consensus
func (s *Server) SetConsensus(c consensus.Consensus) {
	s.consensus = c

	/*
		if handler, ok := s.consensus.(consensus.Handler); ok {
			handler.SetBroadcaster(s)
		}
	*/
}

// GetPeers returns a copy of list of peers
func (s *Server) GetPeers() []string {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	ids := []string{}
	for id := range s.peers {
		ids = append(ids, id)
	}
	return ids
}

func (s *Server) buildInfo() {
	info := &Info{
		Client: s.Name,
		Enode:  s.Enode,
	}

	for _, p := range s.backends {
		cap := &Capability{
			Protocol: *p,
		}

		info.Capabilities = append(info.Capabilities, cap)
	}
	s.info = info
}

func (s *Server) Enqueue(id string, block *types.Block) {
	//s.fetcher.Enqueue(id, block)
}

func (s *Server) FindPeers(targets map[types.Address]bool) map[types.Address]consensus.Peer {
	m := make(map[types.Address]consensus.Peer)
	for _, p := range s.peers {
		pubKey, err := p.Enode.PublicKey()
		if err != nil {
			continue
		}

		addr := crypto.PubKeyToAddress(pubKey)
		if targets[addr] {
			m[addr] = p
		}
	}
	return m
}

// Schedule starts all the tasks once all the protocols have been loaded
func (s *Server) Schedule() error {
	// bootstrap peers
	storedPeers, err := s.peerStore.Load()
	if err != nil {
		return err
	}
	for _, peer := range storedPeers {
		s.Dial(peer)
	}

	// Create rlpx info
	s.buildInfo()

	config := map[string]interface{}{
		"addr": s.config.BindAddress,
		"port": s.config.BindPort,
	}

	if err := s.transport.Setup(s.key, s.backends, s.info, config); err != nil {
		return err
	}

	go func() {
		for {
			session, err := s.transport.Accept()
			if err == nil {
				go func() {
					if err := s.addSession(session); err != nil {
						s.logger.Trace("failed adding session", err)
					}
				}()
			}
		}
	}()

	// Start discovery process
	s.Discovery.Schedule()

	go s.dialRunner()
	return nil
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
	// s.logger.Printf("Dial task %s running", id)

	for {
		select {
		case task := <-tasks:
			//s.logger.Trace("DIAL", "id", id, "task", task)

			err := s.connect(task)

			contains := s.dispatcher.Contains(task)
			busy := false
			if err != nil {
				//s.logger.Trace("Err", "id", id, "err", err)

				if err.Error() == "too many peers" {
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

			if err == nil {
				// update the peerstore
				s.peerStore.Update(task, 0)
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

	// sendToTask("enode://ce661f71a50ff8cce940095c3770beddab79233672366d8244e996f5c1e6e9abc0d5713f13b2de257532d91a64c3c2c952f2b016c895942694c2beaeb3978aa6@127.0.0.1:30303")

	for {
		select {
		case enode := <-s.addPeer:
			sendToTask(enode)

		case enode := <-s.Discovery.Deliver():
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

// GetPeerByPrefix searches a peer by his prefix
func (s *Server) GetPeerByPrefix(search string) (*Peer, bool) {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	for id, peer := range s.peers {
		if strings.HasPrefix(id, search) {
			return peer, true
		}
	}
	return nil, false
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

func (s *Server) connectWithEnode(rawURL string) error {
	if _, ok := s.peers[rawURL]; ok {
		// TODO: add tests
		// Trying to connect with an already connected id
		// TODO, after disconnect do we remove the peer from this list?
		return nil
	}

	session, err := s.transport.DialTimeout(rawURL, defaultDialTimeout)
	if err != nil {
		return err
	}

	// match protocols
	return s.addSession(session)
}

func (s *Server) addSession(session Session) error {
	p := newPeer(session)

	instances := []*Instance{}
	var instanceLock sync.Mutex

	streams := session.Streams()
	errs := make(chan error, len(streams))

	for _, stream := range streams {
		go func(stream Stream) {
			spec := stream.Protocol()

			proto, ok := s.getProtocol(spec.Name, spec.Version)
			if !ok {
				// This should not happen, its an internal error
				errs <- fmt.Errorf("protocol does not exists")
				return
			}

			handler, err := proto.HandlerFn(stream, p)
			if err != nil {
				errs <- err
				return
			}

			instanceLock.Lock()
			instances = append(instances, &Instance{
				Protocol: proto,
				Handler:  handler,
			})
			instanceLock.Unlock()
			errs <- nil
		}(stream)
	}

	for i := 0; i < len(streams); i++ {
		if err := <-errs; err != nil {
			p.Close()
			return err
		}
	}

	p.protocols = instances

	// Remove peer from list if the session is closed
	go func() {
		<-session.CloseChan()

		s.peersLock.Lock()
		delete(s.peers, p.ID)
		s.peersLock.Unlock()
	}()

	s.peersLock.Lock()
	s.peers[p.ID] = p
	s.peersLock.Unlock()

	select {
	case s.EventCh <- MemberEvent{Type: NodeJoin, Peer: p}:
	default:
	}

	return nil
}

// RegisterProtocol registers a protocol
func (s *Server) RegisterProtocol(b []*Protocol) error {
	s.backends = append(s.backends, b...)
	// TODO, check if the backend is already registered
	return nil
}

func (s *Server) ID() enode.ID {
	return s.Enode.ID
}

func (s *Server) getProtocol(name string, version uint) (*Protocol, bool) {
	for _, p := range s.backends {
		proto := p.Spec
		if proto.Name == name && proto.Version == version {
			return p, true
		}
	}
	return nil, false
}

func (s *Server) Close() {
	// close peers
	for _, i := range s.peers {
		i.Close()
	}

	if err := s.peerStore.Close(); err != nil {
		panic(err)
	}

	// close transport
	if err := s.transport.Close(); err != nil {
		s.logger.Error("failed to close transport", "err", err.Error())
	}
}
