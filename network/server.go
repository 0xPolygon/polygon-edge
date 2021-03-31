package network

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/0xPolygon/minimal/chain"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	noise "github.com/libp2p/go-libp2p-noise"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/multiformats/go-multiaddr"
)

// var _ network.Notifiee = &Server{}

type Config struct {
	NoDiscover bool
	Addr       *net.TCPAddr
	DataDir    string
	MaxPeers   uint64
	Chain      *chain.Chain
}

func DefaultConfig() *Config {
	return &Config{
		NoDiscover: false,
		Addr:       &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1478},
		MaxPeers:   10,
	}
}

type Server struct {
	logger hclog.Logger
	config *Config

	closeCh chan struct{}

	host  host.Host
	addrs []multiaddr.Multiaddr

	peers     map[peer.ID]*Peer
	peersLock sync.Mutex

	updateCh  chan peer.ID
	dialQueue *dialQueue

	identity  *identity
	discovery *discovery

	// map of peers that we are connecting
	//pending sync.Map

	// dht config
	// dht *dht.IpfsDHT

	// pubsub
	ps *pubsub.PubSub

	watcher *peerWatcher
}

type Peer struct {
	srv *Server

	Info peer.AddrInfo
}

func NewServer(logger hclog.Logger, config *Config) (*Server, error) {
	logger = logger.Named("network")

	key, err := ReadLibp2pKey(config.DataDir)
	if err != nil {
		return nil, err
	}
	addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", config.Addr.IP.String(), config.Addr.Port))
	if err != nil {
		return nil, err
	}

	host, err := libp2p.New(
		context.Background(),
		// Use noise as the encryption protocol
		libp2p.Security(noise.ID, noise.New),
		libp2p.ListenAddrs(addr),
		libp2p.Identity(key),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p stack: %v", err)
	}

	srv := &Server{
		logger:    logger,
		config:    config,
		host:      host,
		addrs:     []multiaddr.Multiaddr{addr},
		peers:     map[peer.ID]*Peer{},
		updateCh:  make(chan peer.ID),
		dialQueue: newDialQueue(),
		closeCh:   make(chan struct{}),
	}

	// start identity
	srv.identity = &identity{srv: srv}
	srv.identity.setup()

	go srv.runDial()

	logger.Info("LibP2P server running", "addr", AddrInfoToString(srv.AddrInfo()))

	//fmt.Println(config.MaxPeers)
	//fmt.Println(config.NoDiscover)

	if !config.NoDiscover {
		// start discovery
		srv.discovery = &discovery{srv: srv}
		srv.discovery.setup()
		/*
			if err := srv.setupDHT(context.Background()); err != nil {
				return nil, err
			}
		*/
	}

	// start gossip protocol
	ps, err := pubsub.NewGossipSub(context.Background(), host)
	if err != nil {
		return nil, err
	}
	srv.ps = ps

	// create peer watcher
	sub, err := srv.SubscribePeerEvents()
	if err != nil {
		return nil, err
	}
	srv.watcher = &peerWatcher{sub: sub}
	go srv.watcher.run()

	return srv, nil
}

/*
func (s *Server) setPending(id peer.ID) {
	s.pending.Store(id, true)
}

func (s *Server) isPending(id peer.ID) bool {
	_, ok := s.pending.Load(id)
	return ok
}
*/

/*
func (s *Server) setupDHT(ctx context.Context) error {
	s.logger.Info("start dht discovery")

	nsValidator := record.NamespacedValidator{}
	nsValidator["ipns"] = ipns.Validator{}
	nsValidator["pk"] = record.PublicKeyValidator{}

	d, err := dht.New(ctx, s.host, dht.Mode(dht.ModeServer), dht.Validator(nsValidator), dht.BootstrapPeers())
	if err != nil {
		return err
	}
	if err = d.Bootstrap(ctx); err != nil {
		return err
	}

	s.dht = d
	s.dht.RoutingTable().PeerAdded = func(p peer.ID) {
	}

	return nil
}
*/

const dialSlots = 1 // To be modified later

func (s *Server) runDial() {
	notifyCh := make(chan struct{})
	s.identity.notifyCh = notifyCh

	for {
		slots := int64(s.config.MaxPeers) - (s.numPeers() + s.identity.numPending())
		if slots < 0 {
			slots = 0
		}

		fmt.Println("-- slots --")
		fmt.Println(s.config.MaxPeers, s.numPeers(), s.identity.numPending())
		fmt.Println(slots)

		for i := int64(0); i < slots; i++ {
			tt := s.dialQueue.pop()
			if tt == nil {
				// dial closed
				return
			}

			// dial the task
			s.logger.Debug("dial", "local", s.host.ID(), "addr", tt.addr.String())
			if err := s.host.Connect(context.Background(), *tt.addr); err != nil {
				s.logger.Error("failed to dial", "addr", tt.addr.String(), "err", err)
			}
		}

		// wait until there is a notify
		select {
		case <-notifyCh:
		case <-s.closeCh:
			return
		}
	}
}

func (s *Server) numPeers() int64 {
	return int64(len(s.peers))
}

func (s *Server) addPeer(id peer.ID) {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	p := &Peer{
		srv:  s,
		Info: s.host.Peerstore().PeerInfo(id),
	}
	s.peers[id] = p
}

func (s *Server) delPeer(id peer.ID) {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	delete(s.peers, id)
}

func (s *Server) Disconnect(peer peer.ID, reason string) {
	if s.host.Network().Connectedness(peer) == network.Connected {
		// send some close message
		s.host.Network().ClosePeer(peer)
	}
}

var DefaultJoinTimeout = 10 * time.Second

func (s *Server) JoinAddr(addr string, timeout time.Duration) error {
	addr0, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return err
	}
	addr1, err := peer.AddrInfoFromP2pAddr(addr0)
	if err != nil {
		return err
	}
	return s.Join(addr1, timeout)
}

func (s *Server) Join(addr *peer.AddrInfo, timeout time.Duration) error {
	s.dialQueue.add(addr, 1)

	if timeout == 0 {
		return nil
	}
	err := s.watcher.watch(addr.ID, timeout)
	return err
}

type peerWatcher struct {
	sub event.Subscription

	lock     sync.Mutex
	watchers map[peer.ID]chan error
}

func (p *peerWatcher) watch(peerID peer.ID, dur time.Duration) error {
	ch := make(chan error)

	p.lock.Lock()
	if p.watchers == nil {
		p.watchers = map[peer.ID]chan error{}
	}
	p.watchers[peerID] = ch
	p.lock.Unlock()

	select {
	case <-time.After(dur):
		p.lock.Lock()
		delete(p.watchers, peerID)
		p.lock.Unlock()

		return fmt.Errorf("timeout")
	case err := <-ch:
		return err
	}
}

func (p *peerWatcher) notify(peerID peer.ID, err error) {
	p.lock.Lock()
	ch, ok := p.watchers[peerID]
	if ok {
		ch <- err
		close(ch)
	}
	p.lock.Unlock()
}

func (p *peerWatcher) run() {
	for {
		evnt := <-p.sub.Out()
		switch obj := evnt.(type) {
		case PeerConnectedEvent:
			p.notify(obj.Peer, obj.Err)
		}
	}
}

func (s *Server) Close() {
	s.host.Close()
	s.dialQueue.Close()
	close(s.closeCh)
}

func (s *Server) UpdateCh() chan peer.ID {
	return s.updateCh
}

func (s *Server) StartStream(proto string, id peer.ID) network.Stream {
	stream, err := s.host.NewStream(context.Background(), id, protocol.ID(proto))
	if err != nil {
		panic(err) // TODO
	}
	return stream
}

type Protocol interface {
	Handler() func(network.Stream)
}

func (s *Server) Register(id string, p Protocol) {
	s.wrapStream(id, p.Handler())
}

func (s *Server) wrapStream(id string, handle func(network.Stream)) {
	s.host.SetStreamHandler(protocol.ID(id), func(stream network.Stream) {
		peerID := stream.Conn().RemotePeer()
		s.logger.Trace("open stream", "protocol", id, "peer", peerID)

		handle(stream)
	})
}

func (s *Server) SubscribePeerEvents() (event.Subscription, error) {
	events := []interface{}{
		&PeerConnectedEvent{},
		&PeerDisconnectedEvent{},
	}
	sub, err := s.host.EventBus().Subscribe(events)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (s *Server) AddrInfo() *peer.AddrInfo {
	return &peer.AddrInfo{
		ID:    s.host.ID(),
		Addrs: s.addrs,
	}
}

func StringToAddrInfo(addr string) (*peer.AddrInfo, error) {
	addr0, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return nil, err
	}
	addr1, err := peer.AddrInfoFromP2pAddr(addr0)
	if err != nil {
		return nil, err
	}
	return addr1, nil
}

// AddrInfoToString converts an AddrInfo into a string representation that can be dialed from another node
func AddrInfoToString(addr *peer.AddrInfo) string {
	if len(addr.Addrs) != 1 {
		panic("Not supported")
	}
	return addr.Addrs[0].String() + "/p2p/" + addr.ID.String()
}
