package jsonrpc

import (
	"testing"
	"time"
)

func TestHttp(t *testing.T) {

	startHTTP()
	time.Sleep(50 * time.Second)
}
