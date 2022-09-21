package dial

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
)

func TestDialQueue(t *testing.T) {
	q := NewDialQueue()

	info0 := &peer.AddrInfo{
		ID: peer.ID("a"),
	}
	q.AddTask(info0, 1)
	assert.Equal(t, 1, q.heap.Len())

	info1 := &peer.AddrInfo{
		ID: peer.ID("b"),
	}
	q.AddTask(info1, 1)
	assert.Equal(t, 2, q.heap.Len())

	assert.Equal(t, q.popTaskImpl().addrInfo.ID, peer.ID("a"))
	assert.Equal(t, q.popTaskImpl().addrInfo.ID, peer.ID("b"))
	assert.Equal(t, 0, q.heap.Len())

	assert.Nil(t, q.popTaskImpl())

	done := make(chan struct{})

	go func() {
		q.PopTask()
		done <- struct{}{}
	}()

	// we should not get any peer now
	select {
	case <-done:
		t.Fatal("not expected")
	case <-time.After(1 * time.Second):
	}

	q.AddTask(info0, 1)

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout")
	}
}

func TestDel(t *testing.T) {
	type Action string

	const (
		ActionAdd    Action = "add"
		ActionDelete Action = "delete"
		ActionPop    Action = "pop"
	)

	type task struct {
		id     string
		action Action
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
					action: ActionAdd,
				},
			},
			expectedLen: 1,
		},
		{
			name: "should be able to delete",
			tasks: []task{
				{
					id:     "a",
					action: ActionAdd,
				},
				{
					id:     "a",
					action: ActionDelete,
				},
			},
			expectedLen: 0,
		},
		{
			name: "should succeed on removing non-exist data",
			tasks: []task{
				{
					id:     "a",
					action: ActionAdd,
				},
				{
					id:     "b",
					action: ActionDelete,
				},
			},
			expectedLen: 1,
		},
		{
			name: "should be able to pop",
			tasks: []task{
				{
					id:     "a",
					action: ActionAdd,
				},
				{
					id:     "a",
					action: ActionPop,
				},
			},
			expectedLen: 0,
		},
		{
			name: "should be able to delete popped data",
			tasks: []task{
				{
					id:     "a",
					action: ActionAdd,
				},
				{
					id:     "a",
					action: ActionPop,
				},
				{
					id:     "a",
					action: ActionDelete,
				},
			},
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := NewDialQueue()
			for _, task := range tt.tasks {
				id := peer.ID(task.id)

				switch task.action {
				case ActionAdd:
					q.AddTask(&peer.AddrInfo{
						ID: id,
					}, 1)
				case ActionDelete:
					q.DeleteTask(id)
				case ActionPop:
					d := q.PopTask()
					assert.Equal(t, id, d.addrInfo.ID)
				default:
					t.Errorf("unsupported action: %s", task.action)
				}
			}
			assert.Equal(t, tt.expectedLen, q.heap.Len())
		})
	}
}
