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

type Server struct {
	logger hclog.Logger

	host  host.Host
	addrs []multiaddr.Multiaddr

	peers     map[peer.ID]*Peer
	peersLock sync.Mutex

	updateCh  chan peer.ID
	dialQueue *dialQueue

	identity *identity

	addPeerCh    chan peer.ID
	deletePeerCh chan peer.ID
}

type Peer struct {
	Info peer.AddrInfo
}

func NewServer(logger hclog.Logger, dataDir string, libp2pAddr *net.TCPAddr) (*Server, error) {
	logger = logger.Named("network")

	key, err := readLibp2pKey(dataDir)
	if err != nil {
		return nil, err
	}
	addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", libp2pAddr.IP.String(), libp2pAddr.Port))
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
		host:      host,
		addrs:     []multiaddr.Multiaddr{addr},
		peers:     map[peer.ID]*Peer{},
		updateCh:  make(chan peer.ID),
		dialQueue: newDialQueue(),
	}

	// start identity
	srv.identity = &identity{srv: srv}
	srv.identity.setup()

	// notify for peer connection updates
	host.Network().Notify(srv)
	go srv.runDial()

	logger.Info("LibP2P server running", "addr", AddrInfoToString(srv.AddrInfo()))

	return srv, nil
}

// AddrInfoToString converts an AddrInfo into a string representation that can be dialed from another node
func AddrInfoToString(addr peer.AddrInfo) string {
	if len(addr.Addrs) != 1 {
		panic("Not supported")
	}
	return addr.Addrs[0].String() + "/p2p/" + addr.ID.String()
}

const dialSlots = 1 // To be modified later

func (s *Server) runDial() {
	// we work with dialSlots, there can only be at any time x concurrent slots to dial
	// nodes (this is, connect + identity handshake).

	/*
		dial := func(addr peer.AddrInfo) {
			if err := s.host.Connect(context.Background(), addr); err != nil {
				panic(err)
			}
		}
	*/

	/*
		dialCh := make(chan peer.AddrInfo)

		dialFunc := func() {
			for addr := range dialCh {
				if err := s.host.Connect(context.Background(), addr); err != nil {
					panic(err)
				}
			}
		}

		for i := 0; i < dialSlots; i++ {
			go dialFunc()
		}


		for {
			select {
			case <-s.addPeerCh:

			case <-s.deletePeerCh:

			}
		}*/

	for {
		tt := s.dialQueue.pop()

		s.logger.Debug("dial", "addr", tt.addr.String())

		// dial the task
		if err := s.host.Connect(context.Background(), tt.addr); err != nil {
			panic(err)
		}
	}
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
	s.Connect(*addr1)
	return nil
}

func (s *Server) Connect(addr peer.AddrInfo) error {
	s.dialQueue.add(addr, 1)
	return nil
}

func (s *Server) Close() {
	s.host.Close()
	s.dialQueue.Close()
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
