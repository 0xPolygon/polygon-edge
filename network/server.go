package network

import (
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	noise "github.com/libp2p/go-libp2p-noise"
	"github.com/multiformats/go-multiaddr"
)

var _ network.Notifiee = &Server{}

type Config struct {
	NoDiscover bool
	Addr       *net.TCPAddr
	DataDir    string
	MaxPeers   uint64
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

	host  host.Host
	addrs []multiaddr.Multiaddr

	peers     map[peer.ID]*Peer
	peersLock sync.Mutex

	updateCh  chan peer.ID
	dialQueue *dialQueue

	identity *identity

	addPeerCh    chan Peer // Notifies of new peers being added
	deletePeerCh chan Peer // Notifies of peers being deleted

	dialSlots  int              // The maximum number of peers that can be connected
	dialCancel chan interface{} // Dial manager cancel signal
	serverMux  sync.Mutex       // Mutex for controlling variable access in the server object
}

type Peer struct {
	Info peer.AddrInfo
}

func NewServer(logger hclog.Logger, config *Config) (*Server, error) {
	logger = logger.Named("network")

	key, err := readLibp2pKey(config.DataDir)
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
		dialSlots: 1, // TODO: Think about what his value should initially be
	}

	// start identity
	srv.identity = &identity{srv: srv}
	srv.identity.setup()

	// notify for peer connection updates
	host.Network().Notify(srv)
	go srv.runDial()

	logger.Info("LibP2P server running", "addr", AddrInfoToString(srv.AddrInfo()))

	fmt.Println(config.MaxPeers)
	fmt.Println(config.NoDiscover)

	return srv, nil
}

// AddrInfoToString converts an AddrInfo into a string representation that can be dialed from another node
func AddrInfoToString(addr peer.AddrInfo) string {
	if len(addr.Addrs) != 1 {
		panic("Not supported")
	}
	return addr.Addrs[0].String() + "/p2p/" + addr.ID.String()
}

// dialFromQueue is a helper function to dial an address from the dial queue
func (s *Server) dialFromQueue() error {
	// Guaranteed non blocking if called within specific conditions
	tt := s.dialQueue.pop()

	s.logger.Debug("dial", "addr", tt.addr.String())

	// Dial the task
	if err := s.host.Connect(context.Background(), tt.addr); err != nil {
		return fmt.Errorf("unable to dial address %v", tt.addr.ID.String())
	}

	return nil
}

// SetDialSlots updates the max number of dials for the server
func (s *Server) SetDialSlots(newDialMax int) {
	if newDialMax < 1 {
		return
	}

	s.serverMux.Lock()
	defer s.serverMux.Unlock()

	s.dialSlots = newDialMax
}

// dialHelper is a helper function that attempts to add peers from the dial queue, as long as it's possible
func (s *Server) dialHelper() {
	for s.getPeersListSize() < s.dialSlots && s.dialQueue.size() > 1 {
		// There are empty slots and they can be filled up
		if err := s.dialFromQueue(); err != nil {
			panic(err)
		}
	}
}

// runDial is a go routine that balances the number of connected peers,
// so it never exceeds the limit
func (s *Server) runDial() {

	// Initial dial queue empty attempt
	s.dialHelper()

	for {
		select {
		case <-s.addPeerCh:
			// A new peer has been added, dial if there is an open slot
			s.dialHelper()
		case <-s.deletePeerCh:
			// A peer has been deleted, try to fill the slot
			s.dialHelper()
		case <-s.dialCancel:
			// Cancel signal detected, exit gracefully
			return
		}
	}
}

// getPeersListSize returns the size of the current peers list (Threadsafe)
func (s *Server) getPeersListSize() int {
	s.peersLock.Lock()
	defer s.peersLock.Unlock()

	return len(s.peers)
}

func (s *Server) ConnectAddr(addr string) error {
	addr0, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return err
	}

	addr1, err := peer.AddrInfoFromP2pAddr(addr0)
	if err != nil {
		return err
	}

	if err = s.Connect(*addr1); err != nil {
		return err
	}

	return nil
}

func (s *Server) Connect(addr peer.AddrInfo) error {
	s.dialQueue.add(addr, 1)

	// Alert the dial of a connection
	s.addPeerCh <- Peer{
		Info: addr,
	}

	return nil
}

func (s *Server) Close() {
	_ = s.host.Close()
	s.dialQueue.Close()

	s.dialCancel <- "cancel" // arbitrary message
}

func (s *Server) UpdateCh() chan peer.ID {
	return s.updateCh
}

func (s *Server) addPeer(id peer.ID) {
	// TODO: add to custom peer store
	select {
	case s.updateCh <- id:
	default:
	}
}

func (s *Server) StartStream(proto string, id peer.ID) network.Stream {
	stream, err := s.host.NewStream(context.Background(), id, protocol.ID(proto))
	if err != nil {
		panic(err)
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
		// TODO: Auth peer id, for non-identity protocols, the peer
		// must be authenticated before we can call the handle
		handle(stream)
	})
}

func readLibp2pKey(dataDir string) (crypto.PrivKey, error) {
	if dataDir == "" {
		// use an in-memory key
		priv, _, err := crypto.GenerateKeyPair(crypto.Secp256k1, 256)
		if err != nil {
			return nil, err
		}
		return priv, nil
	}

	path := filepath.Join(dataDir, "libp2p")
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to stat (%s): %v", path, err)
	}
	if os.IsNotExist(err) {
		priv, _, err := crypto.GenerateKeyPair(crypto.Secp256k1, 256)
		if err != nil {
			return nil, err
		}
		buf, err := crypto.MarshalPrivateKey(priv)
		if err != nil {
			return nil, err
		}
		if err := ioutil.WriteFile(path, []byte(hex.EncodeToString(buf)), 0600); err != nil {
			return nil, err
		}
		return priv, nil
	}

	// exists
	raw, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	buf, err := hex.DecodeString(string(raw))
	if err != nil {
		return nil, err
	}
	key, err := crypto.UnmarshalPrivateKey(buf)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (s *Server) AddrInfo() peer.AddrInfo {
	return peer.AddrInfo{
		ID:    s.host.ID(),
		Addrs: s.addrs,
	}
}

// Listen implements the network.Notifiee interface
func (s *Server) Listen(network.Network, multiaddr.Multiaddr) {
}

// ListenClose implements the network.Notifiee interface
func (s *Server) ListenClose(network.Network, multiaddr.Multiaddr) {
}

// Connected implements the network.Notifiee interface
func (s *Server) Connected(net network.Network, conn network.Conn) {
	fmt.Println("-c")
}

// Disconnected implements the network.Notifiee interface
func (s *Server) Disconnected(network.Network, network.Conn) {
	fmt.Println("-e")
}

// OpenedStream implements the network.Notifiee interface
func (s *Server) OpenedStream(network.Network, network.Stream) {
	// TODO: Use this to notify when a peer connected has the same protocol?
}

// ClosedStream implements the network.Notifiee interface
func (s *Server) ClosedStream(network.Network, network.Stream) {
}

// TODO: Gossip.
// TODO: Dial pool (min, max nodes).
// TODO: Dht.
// TODO: Fixed nodes (validator set) that are never replaced.
//      - Node priorities (libp2p?)
// TODO: wrap security.
// TODO: Utils to drop peers (call it from identity or syncer)
// TODO: Peers metadata
// TODO: Identity shares possible private keys and its included as metadata
