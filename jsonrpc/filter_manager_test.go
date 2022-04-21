package jsonrpc

import (
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

func TestFilterLog(t *testing.T) {
	store := newMockStore()

	m := NewFilterManager(hclog.NewNullLogger(), store)
	go m.Run()

	id := m.NewLogFilter(&LogQuery{
		Topics: [][]types.Hash{
			{hash1},
		},
	}, nil)

	store.emitEvent(&mockEvent{
		NewChain: []*mockHeader{
			{
				header: &types.Header{
					Hash: hash1,
				},
				receipts: []*types.Receipt{
					{
						Logs: []*types.Log{
							{
								Topics: []types.Hash{
									hash1,
								},
							},
						},
					},
				},
			},
		},
		OldChain: []*mockHeader{
			{
				header: &types.Header{
					Hash: hash2,
				},
				receipts: []*types.Receipt{
					{
						Logs: []*types.Log{
							{
								Topics: []types.Hash{
									hash1,
								},
							},
						},
					},
				},
			},
		},
	})

	time.Sleep(500 * time.Millisecond)

	if _, fetchErr := m.GetFilterChanges(id); fetchErr != nil {
		t.Fatalf("Unable to get filter changes, %v", fetchErr)
	}
}

func TestFilterBlock(t *testing.T) {
	store := newMockStore()

	m := NewFilterManager(hclog.NewNullLogger(), store)
	go m.Run()

	// add block filter
	id := m.NewBlockFilter(nil)

	// emit two events
	store.emitEvent(&mockEvent{
		NewChain: []*mockHeader{
			{
				header: &types.Header{
					Hash: types.StringToHash("1"),
				},
			},
			{
				header: &types.Header{
					Hash: types.StringToHash("2"),
				},
			},
		},
	})

	store.emitEvent(&mockEvent{
		NewChain: []*mockHeader{
			{
				header: &types.Header{
					Hash: types.StringToHash("3"),
				},
			},
		},
	})

	// we need to wait for the manager to process the data
	time.Sleep(500 * time.Millisecond)

	if _, fetchErr := m.GetFilterChanges(id); fetchErr != nil {
		t.Fatalf("Unable to get filter changes, %v", fetchErr)
	}

	// emit one more event, it should not return the
	// first three hashes
	store.emitEvent(&mockEvent{
		NewChain: []*mockHeader{
			{
				header: &types.Header{
					Hash: types.StringToHash("4"),
				},
			},
		},
	})

	time.Sleep(500 * time.Millisecond)

	if _, fetchErr := m.GetFilterChanges(id); fetchErr != nil {
		t.Fatalf("Unable to get filter changes, %v", fetchErr)
	}
}

func TestFilterTimeout(t *testing.T) {
	store := newMockStore()

	m := NewFilterManager(hclog.NewNullLogger(), store)
	m.timeout = 2 * time.Second

	go m.Run()

	// add block filter
	id := m.NewBlockFilter(nil)

	assert.True(t, m.Exists(id))
	time.Sleep(3 * time.Second)
	assert.False(t, m.Exists(id))
}

func TestFilterWebsocket(t *testing.T) {
	store := newMockStore()

	mock := &mockWsConn{
		msgCh: make(chan []byte, 1),
	}

	m := NewFilterManager(hclog.NewNullLogger(), store)
	go m.Run()

	id := m.NewBlockFilter(mock)

	// we cannot call get filter changes for a websocket filter
	_, err := m.GetFilterChanges(id)
	assert.Equal(t, err, ErrWSFilterDoesNotSupportGetChanges)

	// emit two events
	store.emitEvent(&mockEvent{
		NewChain: []*mockHeader{
			{
				header: &types.Header{
					Hash: types.StringToHash("1"),
				},
			},
		},
	})

	select {
	case <-mock.msgCh:
	case <-time.After(2 * time.Second):
		t.Fatal("bad")
	}
}

type mockWsConn struct {
	msgCh chan []byte
}

func (m *mockWsConn) WriteMessage(messageType int, b []byte) error {
	m.msgCh <- b

	return nil
}

func TestHeadStream(t *testing.T) {
	b := &blockStream{}

	b.push(&types.Header{Hash: types.StringToHash("1")})
	b.push(&types.Header{Hash: types.StringToHash("2")})

	cur := b.Head()

	b.push(&types.Header{Hash: types.StringToHash("3")})
	b.push(&types.Header{Hash: types.StringToHash("4")})

	// get the updates, there are two new entries
	updates, next := cur.getUpdates()

	assert.Equal(t, updates[0].Hash.String(), types.StringToHash("3").String())
	assert.Equal(t, updates[1].Hash.String(), types.StringToHash("4").String())

	// there are no new entries
	updates, _ = next.getUpdates()
	assert.Len(t, updates, 0)
}

type MockClosedWSConnection struct{}

func (m *MockClosedWSConnection) WriteMessage(_messageType int, _data []byte) error {
	return websocket.ErrCloseSent
}

func TestClosedFilterDeletion(t *testing.T) {
	store := newMockStore()

	m := NewFilterManager(hclog.NewNullLogger(), store)

	go m.Run()

	// add block filter
	id := m.NewBlockFilter(&MockClosedWSConnection{})

	assert.True(t, m.Exists(id))

	// event is sent to the filter but writing to connection should fail
	err := m.dispatchEvent(&blockchain.Event{
		NewChain: []*types.Header{
			{
				Hash: types.StringToHash("1"),
			},
		},
	})

	// should not return error when the error is websocket.ErrCloseSen because filter is removed instead
	assert.NoError(t, err)

	// false because filter was removed automatically
	assert.False(t, m.Exists(id))
}
