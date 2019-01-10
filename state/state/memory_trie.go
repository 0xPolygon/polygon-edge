package state

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

type MemoryTrie struct {
	storage map[common.Address][]byte
}

func NewMemoryTrie() *MemoryTrie {
	return &MemoryTrie{
		storage: map[common.Address][]byte{},
	}
}

func (m *MemoryTrie) Get(k []byte) ([]byte, bool) {
	data, ok := m.storage[common.BytesToAddress(k)]
	return data, ok
}

func (m *MemoryTrie) addAccount(addr common.Address, account *Account) {
	data, err := rlp.EncodeToBytes(account)
	if err != nil {
		panic(fmt.Errorf("test encoding failed: %v", err))
	}
	m.storage[addr] = data
}
