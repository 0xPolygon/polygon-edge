package network

import (
	"testing"
	"time"
)

func TestGrpcStream(t *testing.T) {
	srv0 := CreateServer(t, nil)
	srv1 := CreateServer(t, func(c *Config) {
		// c.Chain.Params.ChainID = 10
	})

	// connect with 0 -> 1
	MultiJoin(t, srv0, srv1)

	time.Sleep(5 * time.Second)
}

// Test: Connect maxPeers
