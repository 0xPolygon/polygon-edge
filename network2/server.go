package network2

import (
	"context"

	libp2pgrpc "github.com/0xPolygon/minimal/network2/grpc"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	noise "github.com/libp2p/go-libp2p-noise"
	ma "github.com/multiformats/go-multiaddr"
	"google.golang.org/grpc"
)

/*
TODO:
- Discovery
*/

// Server is the P2P server
type Server struct {
	logger hclog.Logger
	config *Config

	// host is the id of the server
	host host.Host

	// grpcProtocol stub
	grpcServer *libp2pgrpc.GRPCProtocol
}

// Config is the configuration to parametrize the P2P server
type Config struct {
	PrivKey crypto.PrivKey
	Addrs   []ma.Multiaddr
}

// NewServer creates a new P2P server
func NewServer(logger hclog.Logger, config *Config) (*Server, error) {
	s := &Server{
		logger: logger,
		config: config,
	}

	host, _ := libp2p.New(
		context.Background(),
		// Use noise as the encryption protocol
		libp2p.Security(noise.ID, noise.New),
		libp2p.ListenAddrs(config.Addrs...),
		libp2p.Identity(config.PrivKey),
	)

	s.grpcServer = libp2pgrpc.NewGRPCProtocol(context.Background(), host)
	s.host = host
	return s, nil
}

// AddrInfo returns the addr info of the server
func (s *Server) AddrInfo() *peer.AddrInfo {
	return &peer.AddrInfo{
		ID:    s.host.ID(),
		Addrs: s.config.Addrs,
	}
}

// Close stops the P2P server
func (s *Server) Close() {
	s.host.Close()
}

// AddPeerFromAddrInfo adds a peer with the AddrInfo
func (s *Server) AddPeerFromAddrInfo(peer *peer.AddrInfo) {
	s.host.Peerstore().AddAddrs(peer.ID, peer.Addrs, peerstore.PermanentAddrTTL)
}

// AddPeerFromMultiAddr adds a peer with the multiaddr format
func (s *Server) AddPeerFromMultiAddr(addr ma.Multiaddr) error {
	p, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return err
	}
	s.AddPeerFromAddrInfo(p)
	return nil
}

// AddPeerFromMultiAddrString adds a peer in string
func (s *Server) AddPeerFromMultiAddrString(str string) error {
	addr, err := ma.NewMultiaddr(str)
	if err != nil {
		return err
	}
	if err := s.AddPeerFromMultiAddr(addr); err != nil {
		return err
	}
	return nil
}

// Dial returns an GRPC client
func (s *Server) Dial(id peer.ID) (*grpc.ClientConn, error) {
	return s.grpcServer.Dial(context.Background(), id, grpc.WithInsecure(), grpc.WithBlock())
}
