package txpool

import (
	"sync"

	"github.com/0xPolygon/polygon-edge/types"
)

// Lookup map used to find transactions present in the pool
type lookupMap struct {
	sync.RWMutex
	all map[types.Hash]*types.Transaction
}

// add inserts the given transaction into the map. Returns false
// if it already exists. [thread-safe]
func (m *lookupMap) add(tx *types.Transaction) bool {
	m.Lock()
	defer m.Unlock()

	if _, exists := m.all[tx.Hash]; exists {
		return false
	}

	m.all[tx.Hash] = tx

	return true
}

// remove removes the given transactions from the map. [thread-safe]
func (m *lookupMap) remove(txs ...*types.Transaction) {
	m.Lock()
	defer m.Unlock()

	for _, tx := range txs {
		delete(m.all, tx.Hash)
	}
}

// get returns the transaction associated with the given hash. [thread-safe]
func (m *lookupMap) get(hash types.Hash) (*types.Transaction, bool) {
	m.RLock()
	defer m.RUnlock()

	tx, ok := m.all[hash]
	if !ok {
		return nil, false
	}

	return tx, true
}
