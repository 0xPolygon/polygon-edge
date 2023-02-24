package service

import (
	"sync"
)

type AAPool interface {
	Put(string, *AATransaction)
	Pull() *AAPoolTransaction
}

var _ AAPool = (*aaPool)(nil)

type aaPool struct {
	mutex sync.Mutex
	pool  []*AAPoolTransaction
}

func NewAAPool() *aaPool {
	return &aaPool{}
}

func (p *aaPool) Put(id string, tx *AATransaction) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	ptx := &AAPoolTransaction{ID: id, Tx: &tx.Transaction}
	p.pool = append(p.pool, ptx)
}

func (p *aaPool) Pull() *AAPoolTransaction {
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
