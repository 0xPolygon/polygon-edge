package network

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

var initialPort = uint64(2000)

func createServer(t *testing.T, callback func(c *Config)) *Server {
	// create the server
	cfg := DefaultConfig()
	cfg.Addr.Port = int(atomic.AddUint64(&initialPort, 1))

	if callback != nil {
		callback(cfg)
	}
	srv, err := NewServer(hclog.NewNullLogger(), cfg)
	assert.NoError(t, err)

	return srv
}

func TestDhtPeerAdded(t *testing.T) {
	srv0 := createServer(t, nil)
	srv1 := createServer(t, nil)
	srv2 := createServer(t, nil)

	// serial join, srv0 -> srv1 -> srv2
	srv0.Connect(srv1.AddrInfo())
	srv1.Connect(srv2.AddrInfo())

	// wait for the propagation
	time.Sleep(5 * time.Second)

	assert.Len(t, srv0.host.Peerstore().Peers(), 3)
	assert.Len(t, srv1.host.Peerstore().Peers(), 3)
	assert.Len(t, srv2.host.Peerstore().Peers(), 3)
}
