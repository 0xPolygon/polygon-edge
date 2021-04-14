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

	type Request struct {
		from      uint
		to        uint
		isSuccess bool
	}
	type Expected struct {
		numPeers int64
	}

	tests := []struct {
		name     string
		cfgs     []func(*Config)
		requests []Request
		expected []Expected
	}{
		{
			name: "server 1 should reject incoming connection request from server 3 due to limit",
			cfgs: []func(*Config){
				func(c *Config) {
					c.MaxPeers = 1
					c.NoDiscover = true
				},
				func(c *Config) {
					c.MaxPeers = 1
					c.NoDiscover = true
				},
				func(c *Config) {
					c.MaxPeers = 1
					c.NoDiscover = true
				},
			},
			requests: []Request{
				{
					from:      0,
					to:        1,
					isSuccess: true,
				},
				{
					from:      2,
					to:        0,
					isSuccess: false,
				},
			},
			expected: []Expected{
				{
					numPeers: 1,
				},
				{
					numPeers: 1,
				},
				{
					numPeers: 0,
				},
			},
		},
		{
			name: "server 1 should not connect to server 3 due to limit",
			cfgs: []func(*Config){
				func(c *Config) {
					c.MaxPeers = 1
					c.NoDiscover = true
				},
				func(c *Config) {
					c.MaxPeers = 1
					c.NoDiscover = true
				},
				func(c *Config) {
					c.MaxPeers = 1
					c.NoDiscover = true
				},
			},
			requests: []Request{
				{
					from:      0,
					to:        1,
					isSuccess: true,
				},
				{
					from:      0,
					to:        2,
					isSuccess: false,
				},
			},
			expected: []Expected{
				{
					numPeers: 1,
				},
				{
					numPeers: 1,
				},
				{
					numPeers: 0,
				},
			},
		},
		{
			name: "should be success",
			cfgs: []func(*Config){
				func(c *Config) {
					c.MaxPeers = 2
					c.NoDiscover = true
				},
				func(c *Config) {
					c.MaxPeers = 1
					c.NoDiscover = true
				},
				func(c *Config) {
					c.MaxPeers = 1
					c.NoDiscover = true
				},
			},
			requests: []Request{
				{
					from:      0,
					to:        1,
					isSuccess: true,
				},
				{
					from:      0,
					to:        2,
					isSuccess: true,
				},
			},
			expected: []Expected{
				{
					numPeers: 2,
				},
				{
					numPeers: 1,
				},
				{
					numPeers: 1,
				},
			},
		},
	}

	const timeout = time.Second * 10
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvs := make([]*Server, len(tt.cfgs))
			for i, cfg := range tt.cfgs {
				srvs[i] = createServer(t, cfg)
			}
			defer func() {
				for _, s := range srvs {
					s.Close()
				}
			}()

			// Test join
			for _, req := range tt.requests {
				src, dst := srvs[req.from], srvs[req.to]
				err := src.Join(dst.AddrInfo(), timeout)

				if req.isSuccess {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
				}
			}

			// Test result
			for i, ex := range tt.expected {
				assert.Equal(t, srvs[i].numPeers(), ex.numPeers)
			}
		})
	}
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
