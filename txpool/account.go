package txpool

import (
	"sync"
	"sync/atomic"

	"github.com/0xPolygon/polygon-sdk/types"
)

// Thread safe map of all accounts registered by the pool.
// Each account (value) is bound to one address (key).
type accountsMap struct {
	sync.Map
	count uint64
}

// Intializes an account for the given address.
func (m *accountsMap) initOnce(addr types.Address, nonce uint64) *account {
	a, _ := m.LoadOrStore(addr, &account{})
	newAccount := a.(*account)

	// run only once
	newAccount.init.Do(func() {
		// create queues
		newAccount.enqueued = newAccountQueue()
		newAccount.promoted = newAccountQueue()

		// set the nonce
		newAccount.setNonce(nonce)

		// update global count
		atomic.AddUint64(&m.count, 1)
	})

	return newAccount
}

// exists checks if an account exists within the map.
func (m *accountsMap) exists(addr types.Address) bool {
	_, ok := m.Load(addr)
	return ok
}

// Collects the primaries of all accounts.
func (m *accountsMap) getPrimaries() (primaries transactions) {
	m.Range(func(key, value interface{}) bool {
		account := m.get(key.(types.Address))

		account.promoted.lock(false)
		defer account.promoted.unlock()

		// add primary
		if tx := account.promoted.peek(); tx != nil {
			primaries = append(primaries, tx)
		}

		return true
	})

	return primaries
}

// get returns the account associated with the given address.
func (m *accountsMap) get(addr types.Address) *account {
	a, ok := m.Load(addr)
	if !ok {
		return nil
	}

	return a.(*account)
}

// promoted returns the number of all promoted transactons.
func (m *accountsMap) promoted() (total uint64) {
	m.Range(func(key, value interface{}) bool {
		account := m.get(key.(types.Address))

		account.promoted.lock(false)
		defer account.promoted.unlock()

		total += account.promoted.length()
		return true
	})

	return
}

// allTxs returns all promoted and all enqueued transactions, depending on the flag.
func (m *accountsMap) allTxs(includeEnqueued bool) (
	allPromoted, allEnqueued map[types.Address][]*types.Transaction,
) {
	allPromoted = make(map[types.Address][]*types.Transaction)
	allEnqueued = make(map[types.Address][]*types.Transaction)
	m.Range(func(key, value interface{}) bool {
		addr := key.(types.Address)
		account := m.get(addr)

		account.promoted.lock(false)
		defer account.promoted.unlock()

		allPromoted[addr] = account.promoted.queue

		if includeEnqueued {
			account.enqueued.lock(false)
			defer account.enqueued.unlock()

			allEnqueued[addr] = account.enqueued.queue
		}

		return true
	})

	return
}

// An account is the core structure for processing
// transactions from a specific address. The nextNonce
// field is what separetes the enqueued from promoted:
//
// 	1. enqueued - transactions higher than the nextNonce
// 	2. promoted - transactions lower than the nextNonce
//
// If an enqueued transaction matches the nextNonce,
// a promoteRequest is signaled for this account
// indicating the account's enqueued transaction(s)
// are ready to be moved to the promoted queue.
type account struct {
	init               sync.Once
	enqueued, promoted *accountQueue
	nextNonce          uint64
}

// getNonce returns the next expected nonce for this account.
func (a *account) getNonce() uint64 {
	return atomic.LoadUint64(&a.nextNonce)
}

// setNonce sets the next expected nonce for this account.
func (a *account) setNonce(nonce uint64) {
	atomic.StoreUint64(&a.nextNonce, nonce)
}

// enqueue attempts tp push the transaction onto the enqueued queue.
func (a *account) enqueue(tx *types.Transaction, demoted bool) error {
	a.enqueued.lock(true)
	defer a.enqueued.unlock()

	// only accept low nonce if
	// tx was demoted
	if tx.Nonce < a.getNonce() &&
		!demoted {
		return ErrNonceTooLow
	}

	// enqueue tx
	a.enqueued.push(tx)

	return nil
}

// Promote moves eligible transactions from enqueued to promoted.
//
// Eligible transactions are all sequential in order of nonce
// and the first one has to have nonce less (or equal) to the account's
// nextNonce.
func (a *account) promote() (uint64, []*types.Transaction) {
	a.promoted.lock(true)
	a.enqueued.lock(true)
	defer func() {
		a.promoted.unlock()
		a.enqueued.unlock()
	}()

	currentNonce := a.getNonce()
	if a.enqueued.length() == 0 ||
		a.enqueued.peek().Nonce > currentNonce {
		// nothing to promote
		return 0, nil
	}

	promoted := uint64(0)
	promotedTxns := make([]*types.Transaction, 0)
	nextNonce := a.enqueued.peek().Nonce
	for {
		tx := a.enqueued.peek()
		if tx == nil ||
			tx.Nonce != nextNonce {
			break
		}

		// pop from enqueued
		tx = a.enqueued.pop()

		// push to promoted
		a.promoted.push(tx)
		promotedTxns = append(promotedTxns, tx)

		// update counters
		nextNonce += 1
		promoted += 1
	}

	// only update the nonce map if the new nonce
	// is higher than the one previously stored.
	if nextNonce > currentNonce {
		a.setNonce(nextNonce)
	}

	return promoted, promotedTxns
}
