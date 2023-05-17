package forkmanager

import (
	"fmt"
	"sort"
	"sync"
)

/*
Regarding whether it is okay to use the Singleton pattern in Go, it's a topic of some debate.
The Singleton pattern can introduce global state and make testing and concurrent programming more challenging.
It can also make code tightly coupled and harder to maintain.
In general, it's recommended to favor dependency injection and explicit collaboration over singletons.

However, there might be scenarios where the Singleton pattern is still useful,
such as managing shared resources or maintaining a global configuration.
Just be mindful of the potential drawbacks and consider alternative patterns when appropriate.
*/

var (
	forkManagerInstance     *forkManager
	forkManagerInstanceLock sync.Mutex
)

type forkManager struct {
	lock sync.Mutex

	forkMap     map[ForkName]*Fork
	handlersMap map[ForkHandlerName][]ForkActiveHandler
}

func GetInstance() *forkManager {
	forkManagerInstanceLock.Lock()
	defer forkManagerInstanceLock.Unlock()

	if forkManagerInstance == nil {
		forkManagerInstance = &forkManager{
			forkMap:     map[ForkName]*Fork{},
			handlersMap: map[ForkHandlerName][]ForkActiveHandler{},
		}
	}

	return forkManagerInstance
}

func (fm *forkManager) RegisterFork(name ForkName) {
	fm.lock.Lock()
	defer fm.lock.Unlock()

	fm.forkMap[name] = &Fork{
		Name:            name,
		FromBlockNumber: 0,
		IsActive:        false,
		Handlers:        map[ForkHandlerName]interface{}{},
	}
}

func (fm *forkManager) RegisterHandler(forkName ForkName, handlerName ForkHandlerName, handler interface{}) error {
	fm.lock.Lock()
	defer fm.lock.Unlock()

	fork, exists := fm.forkMap[forkName]
	if !exists {
		return fmt.Errorf("fork does not exist: %s", forkName)
	}

	fork.Handlers[handlerName] = handler

	return nil
}

func (fm *forkManager) ActivateFork(forkName ForkName, blockNumber uint64) error {
	fm.lock.Lock()
	defer fm.lock.Unlock()

	fork, exists := fm.forkMap[forkName]
	if !exists {
		return fmt.Errorf("fork does not exist: %s", forkName)
	}

	if fork.IsActive {
		return nil // already activated
	}

	fork.IsActive = true
	fork.FromBlockNumber = blockNumber

	for forkHandlerName, forkHandler := range fork.Handlers {
		fm.addHandler(forkHandlerName, blockNumber, forkHandler)
	}

	return nil
}

func (fm *forkManager) DeactivateFork(forkName ForkName) error {
	fm.lock.Lock()
	defer fm.lock.Unlock()

	fork, exists := fm.forkMap[forkName]
	if !exists {
		return fmt.Errorf("fork does not exist: %s", forkName)
	}

	if !fork.IsActive {
		return nil // already deactivated
	}

	fork.IsActive = false

	for forkHandlerName := range fork.Handlers {
		fm.removeHandler(forkHandlerName, fork.FromBlockNumber)
	}

	return nil
}

func (fm *forkManager) GetHandler(name ForkHandlerName, blockNumber uint64) interface{} {
	fm.lock.Lock()
	defer fm.lock.Unlock()

	handlers, exists := fm.handlersMap[name]
	if !exists {
		panic(fmt.Errorf("handlers not registered for %s", name)) //nolint:gocritic
	}

	// binary search to find first position inside []*ForkHandler where FromBlockNumber >= blockNumber
	pos := sort.Search(len(handlers), func(i int) bool {
		return blockNumber < handlers[i].FromBlockNumber
	}) - 1

	return handlers[pos].Handler
}

func (fm *forkManager) IsForkSupported(name ForkName) bool {
	fm.lock.Lock()
	defer fm.lock.Unlock()

	_, exists := fm.forkMap[name]

	return exists
}

func (fm *forkManager) IsForkEnabled(name ForkName, blockNumber uint64) bool {
	fm.lock.Lock()
	defer fm.lock.Unlock()

	fork, exists := fm.forkMap[name]
	if !exists {
		return false
	}

	return fork.IsActive && fork.FromBlockNumber <= blockNumber
}

func (fm *forkManager) GetForkBlock(name ForkName) (uint64, error) {
	fm.lock.Lock()
	defer fm.lock.Unlock()

	fork, exists := fm.forkMap[name]
	if !exists {
		return 0, fmt.Errorf("fork does not exists: %s", name)
	}

	if !fork.IsActive {
		return 0, fmt.Errorf("fork is not active: %s", name)
	}

	return fork.FromBlockNumber, nil
}

func (fm *forkManager) addHandler(handlerName ForkHandlerName, blockNumber uint64, handler interface{}) {
	if handlers, exists := fm.handlersMap[handlerName]; !exists {
		fm.handlersMap[handlerName] = []ForkActiveHandler{
			{
				FromBlockNumber: blockNumber,
				Handler:         handler,
			},
		}
	} else {
		// keep everything in sorted order
		index := sort.Search(len(handlers), func(i int) bool {
			return handlers[i].FromBlockNumber >= blockNumber
		})
		handlers = append(handlers, ForkActiveHandler{})
		copy(handlers[index+1:], handlers[index:])
		handlers[index] = ForkActiveHandler{
			FromBlockNumber: blockNumber,
			Handler:         handler,
		}
		fm.handlersMap[handlerName] = handlers
	}
}

func (fm *forkManager) removeHandler(handlerName ForkHandlerName, blockNumber uint64) {
	handlers, exists := fm.handlersMap[handlerName]
	if !exists {
		return
	}

	index := sort.Search(len(handlers), func(i int) bool {
		return handlers[i].FromBlockNumber == blockNumber
	})

	if index != -1 {
		copy(handlers[index:], handlers[index+1:])
		handlers[len(handlers)-1] = ForkActiveHandler{}
		handlers = handlers[:len(handlers)-1]
	}
}

func (fm *forkManager) RegisterAll(
	availableForks []ForkName,
	handlers []ForkHandler,
	activeForks []*ForkInfo,
) error {
	for _, x := range availableForks {
		fm.RegisterFork(x)
	}

	for _, x := range handlers {
		if err := fm.RegisterHandler(x.ForkName, x.HandlerName, x.Handler); err != nil {
			return err
		}
	}

	for _, it := range activeForks {
		if err := fm.ActivateFork(it.Name, it.FromBlockNumber); err != nil {
			return err
		}
	}

	return nil
}
