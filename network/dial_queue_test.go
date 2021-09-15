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
	assert.Equal(t, 1, q.heap.Len())

	info1 := &peer.AddrInfo{
		ID: peer.ID("b"),
	}
	q.add(info1, 1)
	assert.Equal(t, 2, q.heap.Len())

	assert.Equal(t, q.popImpl().addr.ID, peer.ID("a"))
	assert.Equal(t, q.popImpl().addr.ID, peer.ID("b"))
	assert.Equal(t, 0, q.heap.Len())

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

func TestDel(t *testing.T) {
	type task struct {
		id     string
		action string // "add"/"delete"/"pop"
	}

	tests := []struct {
		name        string
		tasks       []task
		expectedLen int
	}{
		{
			name: "should be able to push element",
			tasks: []task{
				{
					id:     "a",
					action: "add",
				},
			},
			expectedLen: 1,
		},
		{
			name: "should be able to delete",
			tasks: []task{
				{
					id:     "a",
					action: "add",
				},
				{
					id:     "a",
					action: "delete",
				},
			},
			expectedLen: 0,
		},
		{
			name: "should succeed on removing non-exist data",
			tasks: []task{
				{
					id:     "a",
					action: "add",
				},
				{
					id:     "b",
					action: "delete",
				},
			},
			expectedLen: 1,
		},
		{
			name: "should be able to pop",
			tasks: []task{
				{
					id:     "a",
					action: "add",
				},
				{
					id:     "a",
					action: "pop",
				},
			},
			expectedLen: 0,
		},
		{
			name: "should be able to delete popped data",
			tasks: []task{
				{
					id:     "a",
					action: "add",
				},
				{
					id:     "a",
					action: "pop",
				},
				{
					id:     "a",
					action: "delete",
				},
			},
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := newDialQueue()
			for _, task := range tt.tasks {
				id := peer.ID(task.id)

				switch task.action {
				case "add":
					q.add(&peer.AddrInfo{
						ID: id,
					}, 1)
				case "delete":
					q.del(id)
				case "pop":
					d := q.pop()
					assert.Equal(t, id, d.addr.ID)
				default:
					t.Errorf("unsupported action: %s", task.action)
				}
			}
			assert.Equal(t, tt.expectedLen, q.heap.Len())
		})
	}
}
