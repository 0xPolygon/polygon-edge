package jsonrpc

import (
	"errors"
	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"math/big"
	"sync"
)

type mockAccount struct {
	address types.Address
	code    []byte
	account *state.Account
	storage map[types.Hash][]byte
}

func (m *mockAccount) Storage(k types.Hash, v []byte) {
	m.storage[k] = v
}

func (m *mockAccount) Code(code []byte) {
	codeHash := types.BytesToHash(m.address.Bytes())
	m.code = code
	m.account.CodeHash = codeHash.Bytes()
}

func (m *mockAccount) Nonce(n uint64) {
	m.account.Nonce = n
}

func (m *mockAccount) Balance(n uint64) {
	m.account.Balance = new(big.Int).SetUint64(n)
}

type mockHeader struct {
	header   *types.Header
	receipts []*types.Receipt
}

type mockEvent struct {
	OldChain []*mockHeader
	NewChain []*mockHeader
}

type mockStore struct {
	JSONRPCStore

	header       *types.Header
	subscription *blockchain.MockSubscription
	receiptsLock sync.Mutex
	receipts     map[types.Hash][]*types.Receipt
	accounts     map[types.Address]*state.Account
}

func newMockStore() *mockStore {
	return &mockStore{
		header:       &types.Header{Number: 0},
		subscription: blockchain.NewMockSubscription(),
		accounts:     map[types.Address]*state.Account{},
	}
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

func (m *mockStore) GetAccount(root types.Hash, addr types.Address) (*state.Account, error) {
	if acc, ok := m.accounts[addr]; ok {
		return acc, nil
	}

	return nil, errors.New("given root and slot not found in storage")
}

func (m *mockStore) SetAccount(addr types.Address, account *state.Account) {
	m.accounts[addr] = account
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

func (m *mockStore) SubscribeEvents() blockchain.Subscription {
	return m.subscription
}

func (m *mockStore) GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool) {
	return nil, false
}

func (m *mockStore) GetBlockByNumber(num uint64, full bool) (*types.Block, bool) {
	return nil, false
}

func (m *mockStore) GetTxs(inclQueued bool) (
	map[types.Address][]*types.Transaction,
	map[types.Address][]*types.Transaction,
) {
	return nil, nil
}

func (m *mockStore) GetCapacity() (uint64, uint64) {
	return 0, 0
}
