package network

import (
	"testing"
	"time"
)

func TestGrpcStream(t *testing.T) {
	srv0 := CreateServer(t, nil)
	srv1 := CreateServer(t, nil)

	// connect with 0 -> 1
	MultiJoin(t, srv0, srv1)

	time.Sleep(5 * time.Second)
}

// Test: Connect maxPeers
