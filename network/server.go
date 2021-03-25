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
	host  host.Host
	addrs []multiaddr.Multiaddr

	peers     map[peer.ID]*Peer
	peersLock sync.Mutex
}

type Peer struct {
	Info peer.AddrInfo
}

func NewServer(dataDir string, libp2pAddr *net.TCPAddr) (*Server, error) {
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
		host:  host,
		addrs: []multiaddr.Multiaddr{addr},
		peers: map[peer.ID]*Peer{},
	}

	// notify for peer connection updates
	host.Network().Notify(srv)

	return srv, nil
}

func (s *Server) Connect(addr peer.AddrInfo) (*Peer, error) {
	if err := s.host.Connect(context.Background(), addr); err != nil {
		return nil, err
	}
	return nil, nil
}

type Protocol interface {
	GetID() string
	Handler() func(network.Stream)
}

func (s *Server) Register(p Protocol) {
	s.host.SetStreamHandler(protocol.ID(p.GetID()), p.Handler())
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
func (s *Server) Connected(network.Network, network.Conn) {
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
