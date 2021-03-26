package network

import (
	"net"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

func TestGrpcStream(t *testing.T) {
	port := 2000

	createServer := func() *Server {
		// create the server
		srv, err := NewServer(hclog.NewNullLogger(), "", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
		assert.NoError(t, err)
		port++

		return srv
	}

	srv0 := createServer()
	srv1 := createServer()

	// connect with 0 -> 1
	err := srv0.Connect(srv1.AddrInfo())
	assert.NoError(t, err)

	select {
	case <-srv0.updateCh:
	case <-time.After(2 * time.Second):
	}
}
