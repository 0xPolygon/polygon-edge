package minimal

import (
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	libp2pgrpc "github.com/0xPolygon/minimal/helper/grpc"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	noise "github.com/libp2p/go-libp2p-noise"
	ma "github.com/multiformats/go-multiaddr"
)

func (s *Server) setupLibP2P() error {
	// read libp2p key
	key, err := readLibp2pKey(filepath.Join(s.config.DataDir, "keystore", "libp2p"))
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
		libp2p.Identity(key),
	)
	if err != nil {
		return fmt.Errorf("failed to create libp2p stack: %v", err)
	}

	s.libp2pServer = libp2pgrpc.NewGRPCProtocol(context.Background(), host)
	s.host = host

	s.logger.Info("LibP2P server running", "addr", addr.String())
	return nil
}

func readLibp2pKey(path string) (crypto.PrivKey, error) {
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("Failed to stat (%s): %v", path, err)
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
