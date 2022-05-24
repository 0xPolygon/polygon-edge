package txpool

import (
	"sync"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/types"
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
	newAccount := a.(*account) // nolint:forcetypeassert
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

// getPrimaries collects the heads (first-in-line transaction)
// from each of the promoted queues.
func (m *accountsMap) getPrimaries() (primaries []*types.Transaction) {
	m.Range(func(key, value interface{}) bool {
		addressKey, ok := key.(types.Address)
		if !ok {
			return false
		}

		account := m.get(addressKey)

		account.promoted.lock(false)
		defer account.promoted.unlock()

		// add head of the queue
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

	fetchedAccount, ok := a.(*account)
	if !ok {
		return nil
	}

	return fetchedAccount
}

// promoted returns the number of all promoted transactons.
func (m *accountsMap) promoted() (total uint64) {
	m.Range(func(key, value interface{}) bool {
		accountKey, ok := key.(types.Address)
		if !ok {
			return false
		}

		account := m.get(accountKey)

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
		addr, _ := key.(types.Address)
		account := m.get(addr)

		account.promoted.lock(false)
		defer account.promoted.unlock()

		if account.promoted.length() != 0 {
			allPromoted[addr] = account.promoted.queue
		}

		if includeEnqueued {
			account.enqueued.lock(false)
			defer account.enqueued.unlock()

			if account.enqueued.length() != 0 {
				allEnqueued[addr] = account.enqueued.queue
			}
		}

		return true
	})

	return
}

// An account is the core structure for processing
// transactions from a specific address. The nextNonce
// field is what separates the enqueued from promoted transactions:
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
	demotions          uint
}

// getNonce returns the next expected nonce for this account.
func (a *account) getNonce() uint64 {
	return atomic.LoadUint64(&a.nextNonce)
}

// setNonce sets the next expected nonce for this account.
func (a *account) setNonce(nonce uint64) {
	atomic.StoreUint64(&a.nextNonce, nonce)
}

//	reset aligns the account with the new nonce
//	by pruning all transactions with nonce lesser than new.
//	After pruning, a promotion may be signaled if the first
// 	enqueued transaction matches the new nonce.
func (a *account) reset(nonce uint64, promoteCh chan<- promoteRequest) (
	prunedPromoted,
	prunedEnqueued []*types.Transaction,
) {
	a.promoted.lock(true)
	defer a.promoted.unlock()

	//	prune the promoted txs
	prunedPromoted = append(
		prunedPromoted,
		a.promoted.prune(nonce)...,
	)

	if nonce <= a.getNonce() {
		// only the promoted queue needed pruning
		return
	}

	a.enqueued.lock(true)
	defer a.enqueued.unlock()

	//	prune the enqueued txs
	prunedEnqueued = append(
		prunedEnqueued,
		a.enqueued.prune(nonce)...,
	)

	//	update nonce expected for this account
	a.setNonce(nonce)

	//	it is important to signal promotion while
	//	the locks are held to ensure no other
	//	handler will mutate the account
	if first := a.enqueued.peek(); first != nil &&
		first.Nonce == nonce {
		// first enqueued tx is expected -> signal promotion
		promoteCh <- promoteRequest{account: first.From}
	}

	return
}

// enqueue attempts tp push the transaction onto the enqueued queue.
func (a *account) enqueue(tx *types.Transaction) error {
	a.enqueued.lock(true)
	defer a.enqueued.unlock()

	// reject low nonce tx
	if tx.Nonce < a.getNonce() {
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
func (a *account) promote() []*types.Transaction {
	a.promoted.lock(true)
	a.enqueued.lock(true)

	defer func() {
		a.enqueued.unlock()
		a.promoted.unlock()
	}()

	//	sanity check
	currentNonce := a.getNonce()
	if a.enqueued.length() == 0 ||
		a.enqueued.peek().Nonce > currentNonce {
		// nothing to promote
		return nil
	}

	promoted := make([]*types.Transaction, 0)
	nextNonce := a.enqueued.peek().Nonce

	//	move all promotable txs (enqueued txs that are sequential in nonce)
	//	to the account's promoted queue
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

		// update counters
		nextNonce += 1

		// update return result
		promoted = append(promoted, tx)
	}

	// only update the nonce map if the new nonce
	// is higher than the one previously stored.
	if nextNonce > currentNonce {
		a.setNonce(nextNonce)
	}

	return promoted
}
