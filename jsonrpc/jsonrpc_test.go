package jsonrpc

import (
	"fmt"
	"net"
	"testing"

	"github.com/hashicorp/go-hclog"
)

func TestHTTPServer(t *testing.T) {
	store := newMockStore()
	config := &Config{
		Store: store,
		Addr:  &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8545},
	}
	srv, err := NewJSONRPC(hclog.NewNullLogger(), config)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(srv)
}
