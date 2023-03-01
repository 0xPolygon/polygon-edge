package service

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
)

// AATxState defines the interface for a stateful representation of Account Abstraction (AA) transactions
type AATxState interface {
	// Add adds a new AA transaction to the state (database) and returns a wrapper object
	Add(*AATransaction) (*AAStateTransaction, error)
	// Get retrieves the metadata for the AA transaction with the specified ID from the state
	Get(string) (*AAStateTransaction, error)
	// Update modifies the metadata for the AA transaction with the specified ID in the state
	Update(string, func(tx *AAStateTransaction)) error
}

var _ AATxState = (*aaTxState)(nil)

type aaTxState struct {
	// TODO: will be replaced with boldDB in next PR/task
	mutex sync.RWMutex
	items map[string]*AAStateTransaction
}

func NewAATxState() (*aaTxState, error) {
	return &aaTxState{
		items: make(map[string]*AAStateTransaction),
	}, nil
}

func (s *aaTxState) Add(tx *AATransaction) (*AAStateTransaction, error) {
	s.mutex.Lock()
	s.mutex.Unlock()

	// TODO: bolDB implementation will be in next PR/task
	id := uuid.NewString()
	ntx := &AAStateTransaction{
		ID:     id,
		Tx:     tx,
		Status: StatusPending,
	}
	s.items[id] = ntx

	return ntx, nil
}

func (s *aaTxState) Get(id string) (*AAStateTransaction, error) {
	s.mutex.RLock()
	s.mutex.RUnlock()

	// TODO: bolDB implementation will be in next PR/task
	return s.items[id], nil
}

func (s *aaTxState) Update(id string, fn func(tx *AAStateTransaction)) error {
	s.mutex.Lock()
	s.mutex.Unlock()

	// TODO: bolDB implementation will be in next PR/task
	if s.items[id] == nil {
		return fmt.Errorf("tx does not exist: %s", id)
	}

	fn(s.items[id])

	return nil
}
