package service

import (
	"sync"
)

// AAPool defines the interface for a pool of Account Abstraction (AA) transactions
type AAPool interface {
	// Push adds an AA transaction to the pool, associating it with the given account ID
	Push(*AAStateTransaction)
	// Pop removes the next transaction from the pool and returns a wrapper object containing the transaction
	Pop() *AAStateTransaction
	// Init initializes the pool with a set of existing AA transactions. Used on client startup
	Init([]*AAStateTransaction)
}

var _ AAPool = (*aaPool)(nil)

type aaPool struct {
	mutex sync.Mutex
	pool  []*AAStateTransaction
}

func NewAAPool() *aaPool {
	return &aaPool{}
}

func (p *aaPool) Push(tx *AAStateTransaction) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.pool = append(p.pool, tx)
}

func (p *aaPool) Pop() *AAStateTransaction {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	cnt := len(p.pool)

	if cnt == 0 {
		return nil
	}

	item := p.pool[cnt-1]
	p.pool[cnt-1] = nil
	p.pool = p.pool[:cnt-1]

	return item
}

func (p *aaPool) Init([]*AAStateTransaction) {
}
