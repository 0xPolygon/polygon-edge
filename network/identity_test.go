package network

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGrpcStream(t *testing.T) {
	srv0 := createServer(t, nil)
	srv1 := createServer(t, nil)

	// connect with 0 -> 1
	err := srv0.Connect(srv1.AddrInfo())
	assert.NoError(t, err)

	select {
	case <-srv0.updateCh:
	case <-time.After(2 * time.Second):
	}
}
