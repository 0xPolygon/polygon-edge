package jsonrpc

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestEth_TxnPool_SendRawTransaction(t *testing.T) {
	store := &mockStoreTxn{}
	eth := newTestEthEndpoint(store)

	txn := &types.Transaction{
		From: addr0,
		V:    big.NewInt(1),
	}
	txn.ComputeHash()

	data := txn.MarshalRLP()
	_, err := eth.SendRawTransaction(hex.EncodeToHex(data))
	assert.NoError(t, err)
	assert.NotEqual(t, store.txn.Hash, types.ZeroHash)

	// the hash in the txn pool should match the one we send
	if txn.Hash != store.txn.Hash {
		t.Fatal("bad")
	}
}

func TestEth_TxnPool_SendTransaction(t *testing.T) {
	store := &mockStoreTxn{}
	store.AddAccount(addr0)
	eth := newTestEthEndpoint(store)

	txToSend := &types.Transaction{
		From:     addr0,
		To:       argAddrPtr(addr0),
		Nonce:    uint64(0),
		GasPrice: big.NewInt(int64(1)),
	}

	_, err := eth.SendRawTransaction(hex.EncodeToHex(txToSend.MarshalRLP()))
	assert.NoError(t, err)
	assert.NotEqual(t, store.txn.Hash, types.ZeroHash)
}

type mockStoreTxn struct {
	ethStore
	accounts map[types.Address]*mockAccount
	txn      *types.Transaction
}

func (m *mockStoreTxn) AddTx(tx *types.Transaction) error {
	m.txn = tx

	return nil
}

func (m *mockStoreTxn) GetNonce(addr types.Address) uint64 {
	return 1
}
func (m *mockStoreTxn) AddAccount(addr types.Address) *mockAccount {
	if m.accounts == nil {
		m.accounts = map[types.Address]*mockAccount{}
	}

	acct := &mockAccount{
		address: addr,
		account: &state.Account{},
		storage: make(map[types.Hash][]byte),
	}
	m.accounts[addr] = acct

	return acct
}

func (m *mockStoreTxn) Header() *types.Header {
	return &types.Header{}
}

func (m *mockStoreTxn) GetAccount(root types.Hash, addr types.Address) (*state.Account, error) {
	acct, ok := m.accounts[addr]
	if !ok {
		return nil, ErrStateNotFound
	}

	return acct.account, nil
}
