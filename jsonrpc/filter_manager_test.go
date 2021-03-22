package jsonrpc

import (
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

func TestFilterLog(t *testing.T) {
	store := newMockStore()

	m := NewFilterManager(hclog.NewNullLogger(), store)
	go m.Run()

	id := m.addFilter(&LogFilter{
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

	m.GetFilterChanges(id)
}

func TestFilterBlock(t *testing.T) {
	store := newMockStore()

	m := NewFilterManager(hclog.NewNullLogger(), store)
	go m.Run()

	// add block filter
	id := m.addFilter(nil, nil)

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

	m.GetFilterChanges(id)

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

	m.GetFilterChanges(id)
}

func TestFilterTimeout(t *testing.T) {
	store := newMockStore()

	m := NewFilterManager(hclog.NewNullLogger(), store)
	m.timeout = 2 * time.Second

	go m.Run()

	// add block filter
	id := m.addFilter(nil, nil)

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
	assert.Equal(t, err, errFilterDoesNotExists)

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

func (m *mockWsConn) WriteMessage(b []byte) error {
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

type mockStore struct {
	nullBlockchainInterface

	header       *types.Header
	subscription *blockchain.MockSubscription
	receiptsLock sync.Mutex
	receipts     map[types.Hash][]*types.Receipt
}

func (m *mockStore) ApplyTxn(header *types.Header, txn *types.Transaction) ([]byte, bool, error) {
	panic("implement me")
}

func (m *mockStore) GetAccount(root types.Hash, addr types.Address) (*state.Account, error) {
	panic("implement me")
}

func (m *mockStore) GetStorage(root types.Hash, addr types.Address, slot types.Hash) ([]byte, error) {
	panic("implement me")
}

func (m *mockStore) GetCode(hash types.Hash) ([]byte, error) {
	panic("implement me")
}

func newMockStore() *mockStore {
	return &mockStore{
		header:       &types.Header{Number: 0},
		subscription: blockchain.NewMockSubscription(),
	}
}

type mockHeader struct {
	header   *types.Header
	receipts []*types.Receipt
}

type mockEvent struct {
	OldChain []*mockHeader
	NewChain []*mockHeader
}

func (m *mockStore) emitEvent(evnt *mockEvent) {
	if m.receipts == nil {
		m.receipts = map[types.Hash][]*types.Receipt{}
	}

	bEvnt := &blockchain.Event{
		NewChain: []*types.Header{},
		OldChain: []*types.Header{},
	}
	for _, i := range evnt.NewChain {
		m.receipts[i.header.Hash] = i.receipts
		bEvnt.NewChain = append(bEvnt.NewChain, i.header)
	}
	for _, i := range evnt.OldChain {
		m.receipts[i.header.Hash] = i.receipts
		bEvnt.OldChain = append(bEvnt.OldChain, i.header)
	}
	m.subscription.Push(bEvnt)
}

func (m *mockStore) Header() *types.Header {
	return m.header
}

func (m *mockStore) GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error) {
	m.receiptsLock.Lock()
	defer m.receiptsLock.Unlock()

	receipts := m.receipts[hash]
	return receipts, nil
}

// Subscribe subscribes for chain head events
func (m *mockStore) SubscribeEvents() blockchain.Subscription {
	return m.subscription
}
