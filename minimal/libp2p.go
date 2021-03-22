package minimal

import (
	"context"
	"fmt"
	"path/filepath"

	libp2pgrpc "github.com/0xPolygon/minimal/helper/grpc"
	"github.com/0xPolygon/minimal/helper/keystore"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	noise "github.com/libp2p/go-libp2p-noise"
	ma "github.com/multiformats/go-multiaddr"
	"google.golang.org/grpc"
)

func (s *Server) setupLibP2P() error {
	libp2pkey, err := keystore.ReadLibp2pKey(filepath.Join(s.config.DataDir, "keystore", "libp2p"))
	if err != nil {
		return err
	}
	addr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", s.config.LibP2PAddr.IP.String(), s.config.LibP2PAddr.Port))
	if err != nil {
		return err
	}
	s.addrs = []ma.Multiaddr{addr}

	host, err := libp2p.New(
		context.Background(),
		// Use noise as the encryption protocol
		libp2p.Security(noise.ID, noise.New),
		libp2p.ListenAddrs(s.addrs...),
		libp2p.Identity(libp2pkey),
	)
	if err != nil {
		return fmt.Errorf("failed to create libp2p stack: %v", err)
	}

	s.libp2pServer = libp2pgrpc.NewGRPCProtocol(context.Background(), host)
	s.host = host

	s.logger.Info("LibP2P server running", "addr", AddrInfoToString(s.AddrInfo()))
	return nil
}

// AddrInfoToString converts an AddrInfo into a string representation that can be dialed from another node
func AddrInfoToString(addr *peer.AddrInfo) string {
	if len(addr.Addrs) != 1 {
		panic("Not supported")
	}
	return addr.Addrs[0].String() + "/p2p/" + addr.ID.String()
}

// AddrInfo returns the addr info of the server
func (s *Server) AddrInfo() *peer.AddrInfo {
	return &peer.AddrInfo{
		ID:    s.host.ID(),
		Addrs: s.addrs,
	}
}

// AddPeerFromAddrInfo adds a peer with the AddrInfo
func (s *Server) AddPeerFromAddrInfo(peer *peer.AddrInfo) {
	s.host.Peerstore().AddAddrs(peer.ID, peer.Addrs, peerstore.PermanentAddrTTL)
}

// AddPeerFromMultiAddr adds a peer with the multiaddr format
func (s *Server) AddPeerFromMultiAddr(addr ma.Multiaddr) (peer.ID, error) {
	p, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return "", err
	}
	s.AddPeerFromAddrInfo(p)
	return p.ID, nil
}

// AddPeerFromMultiAddrString adds a peer in string
func (s *Server) AddPeerFromMultiAddrString(str string) (peer.ID, error) {
	addr, err := ma.NewMultiaddr(str)
	if err != nil {
		return "", err
	}
	peerID, err := s.AddPeerFromMultiAddr(addr)
	if err != nil {
		return "", err
	}
	return peerID, nil
}

func (s *Server) dial(p peer.ID) (*grpc.ClientConn, error) {

	fmt.Println("-- p --")
	fmt.Println(p.String())

	return s.libp2pServer.Dial(context.Background(), p, grpc.WithInsecure())
}
