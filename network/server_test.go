package network

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
)

func TestDialLifecycle(t *testing.T) {
	// TODO: Lets change this test, instead of testing this with the discovery protocol
	// we should remove discovery protocol and just pipe values to the dialqueue to see how
	// it behaves

	type Request struct {
		from      uint
		to        uint
		isSuccess bool
	}
	defConfig := func(c *Config) {
		c.MaxPeers = 1
		c.NoDiscover = true
	}

	tests := []struct {
		name     string
		cfgs     []func(*Config)
		requests []Request
		// Number of connecting peers after all requests are processed
		numPeers []int64
	}{
		{
			name: "server 1 should reject incoming connection request from server 3 due to limit",
			cfgs: []func(*Config){
				defConfig,
				defConfig,
				defConfig,
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
			numPeers: []int64{
				1, 1, 0,
			},
		},
		{
			name: "server 1 should not connect to server 3 due to limit",
			cfgs: []func(*Config){
				defConfig,
				defConfig,
				defConfig,
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
			numPeers: []int64{
				1, 1, 0,
			},
		},
		{
			name: "should be success",
			cfgs: []func(*Config){
				func(c *Config) {
					defConfig(c)
					c.MaxPeers = 2
				},
				defConfig,
				defConfig,
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
			numPeers: []int64{
				2, 1, 1,
			},
		},
	}

	const timeout = time.Second * 10
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvs := make([]*Server, len(tt.cfgs))
			for i, cfg := range tt.cfgs {
				srvs[i] = CreateServer(t, cfg)
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
			for i, n := range tt.numPeers {
				assert.Equal(t, srvs[i].numPeers(), n)
			}
		})
	}
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
