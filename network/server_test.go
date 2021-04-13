package network

import (
	"testing"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
)

func TestDialLifecycle(t *testing.T) {
	// TODO: Lets change this test, instead of testing this with the discovery protocol
	// we should remove discovery protocol and just pipe values to the dialqueue to see how
	// it behaves

	cfg := func(c *Config) {
		c.MaxPeers = 1
		// c.NoDiscover = true
	}

	srv0 := CreateServer(t, cfg)
	srv1 := CreateServer(t, cfg)
	srv2 := CreateServer(t, cfg)

	// serial join, srv0 -> srv1 -> srv2
	MultiJoin(t,
		srv0, srv1,
		srv1, srv2,
	)

	// since maxPeers=1, the nodes should NOT
	// try to connect to anyone else in the mesh
	assert.Equal(t, srv0.numPeers(), int64(1))
	assert.Equal(t, srv1.numPeers(), int64(2))
	assert.Equal(t, srv2.numPeers(), int64(1))
}

func TestPeerEmitAndSubscribe(t *testing.T) {
	srv0 := CreateServer(t, nil)

	sub, err := srv0.Subscribe()
	assert.NoError(t, err)

	count := 10

	t.Run("serial", func(t *testing.T) {
		for i := 0; i < count; i++ {
			srv0.emitEvent(&PeerEvent{})
			sub.Get()
		}
	})

	t.Run("parallel", func(t *testing.T) {
		for i := 0; i < count; i++ {
			srv0.emitEvent(&PeerEvent{})
		}
		for i := 0; i < count; i++ {
			sub.Get()
		}
	})
}

func TestEncodingPeerAddr(t *testing.T) {
	_, pub, err := crypto.GenerateKeyPair(crypto.Secp256k1, 256)
	assert.NoError(t, err)

	id, err := peer.IDFromPublicKey(pub)
	assert.NoError(t, err)

	addr, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/90")
	assert.NoError(t, err)

	info := &peer.AddrInfo{
		ID:    id,
		Addrs: []multiaddr.Multiaddr{addr},
	}

	str := AddrInfoToString(info)
	info2, err := StringToAddrInfo(str)
	assert.NoError(t, err)
	assert.Equal(t, info, info2)
}

func TestJoinWhenAlreadyConnected(t *testing.T) {
	// if we try to join an already connected node, the watcher
	// should finish as well
	srv0 := CreateServer(t, nil)
	srv1 := CreateServer(t, nil)

	assert.NoError(t, srv0.Join(srv1.AddrInfo(), DefaultJoinTimeout))
	assert.NoError(t, srv0.Join(srv1.AddrInfo(), DefaultJoinTimeout))
}
