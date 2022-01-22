package jsonrpc

import (
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"net"
	"testing"

	"github.com/hashicorp/go-hclog"
)

func TestHTTPServer(t *testing.T) {
	store := newMockStore()
	port, portErr := tests.GetFreePort()

	if portErr != nil {
		t.Fatalf("Unable to fetch free port, %v", portErr)
	}

	config := &Config{
		Store: store,
		Addr:  &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: port},
	}
	_, err := NewJSONRPC(hclog.NewNullLogger(), config)

	if err != nil {
		t.Fatal(err)
	}
}
