package jsonrpc

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
)

func TestHTTPServer(t *testing.T) {
	store := newMockStore()
	config := &Config{
		Store: store,
	}
	srv, err := NewJSONRPC(hclog.NewNullLogger(), config)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(srv)
	time.Sleep(1 * time.Minute)
}
