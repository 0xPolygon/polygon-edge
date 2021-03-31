package network

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
)

func TestDialQueue(t *testing.T) {
	q := newDialQueue()

	info0 := &peer.AddrInfo{
		ID: peer.ID("a"),
	}
	q.add(info0, 1)

	info1 := &peer.AddrInfo{
		ID: peer.ID("b"),
	}
	q.add(info1, 1)

	assert.Equal(t, q.popImpl().addr.ID, peer.ID("a"))
	assert.Equal(t, q.popImpl().addr.ID, peer.ID("b"))
	assert.Nil(t, q.popImpl())

	done := make(chan struct{})
	go func() {
		q.pop()
		done <- struct{}{}
	}()

	// we should not get any peer now
	select {
	case <-done:
		t.Fatal("not expected")
	case <-time.After(1 * time.Second):
	}

	q.add(info0, 1)

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout")
	}
}
