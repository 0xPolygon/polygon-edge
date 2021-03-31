package network

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/chain"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
)

var initialPort = uint64(2000)

func createServer(t *testing.T, callback func(c *Config)) *Server {
	// create the server
	cfg := DefaultConfig()
	cfg.Addr.Port = int(atomic.AddUint64(&initialPort, 1))
	cfg.Chain = &chain.Chain{
		Params: &chain.Params{
			ChainID: 1,
		},
	}

	logger := hclog.New(&hclog.LoggerOptions{
		Name:  "polygon",
		Level: hclog.LevelFromString("debug"),
	})

	if callback != nil {
		callback(cfg)
	}
	srv, err := NewServer(logger, cfg)
	assert.NoError(t, err)

	return srv
}

func multiJoinSerial(t *testing.T, srvs []*Server) {
	dials := []*Server{}
	for i := 0; i < len(srvs)-1; i++ {
		srv, dst := srvs[i], srvs[i+1]
		dials = append(dials, srv, dst)
	}
	multiJoin(t, dials...)
}

func multiJoin(t *testing.T, srvs ...*Server) {
	if len(srvs)%2 != 0 {
		t.Fatal("not an even number")
	}

	doneCh := make(chan error)
	for i := 0; i < len(srvs); i += 2 {
		go func(i int) {
			src, dst := srvs[i], srvs[i+1]
			doneCh <- src.Join(dst.AddrInfo(), 10*time.Second)
		}(i)
	}

	for i := 0; i < len(srvs)/2; i++ {
		err := <-doneCh
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestDialLifecycle(t *testing.T) {
	// TODO: Lets change this test, instead of testing this with the discovery protocol
	// we should remove discovery protocol and just pipe values to the dialqueue to see how
	// it behaves

	cfg := func(c *Config) {
		c.MaxPeers = 1
		// c.NoDiscover = true
	}

	srv0 := createServer(t, cfg)
	srv1 := createServer(t, cfg)
	srv2 := createServer(t, cfg)

	// serial join, srv0 -> srv1 -> srv2
	multiJoin(t,
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
	srv0 := createServer(t, nil)

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

// TODO: Test to get peer notifications.
// It can be used in several places. We should abstract
// as much as possible libp2p events.
// - discovery.
// - peer watch.
// - dial notifications a peer is updated.

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
	srv0 := createServer(t, nil)
	srv1 := createServer(t, nil)

	assert.NoError(t, srv0.Join(srv1.AddrInfo(), DefaultJoinTimeout))
	assert.NoError(t, srv0.Join(srv1.AddrInfo(), DefaultJoinTimeout))
}
