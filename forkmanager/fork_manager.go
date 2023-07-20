package forkmanager

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
)

var (
	forkManagerInstance     *forkManager
	forkManagerInstanceLock sync.Mutex
)

type forkManager struct {
	lock sync.Mutex

	forkMap     map[string]*Fork
	handlersMap map[HandlerDesc][]forkHandler
	params      []forkParamsBlock
}

// GeInstance returns fork manager singleton instance. Thread safe
func GetInstance() *forkManager {
	forkManagerInstanceLock.Lock()
	defer forkManagerInstanceLock.Unlock()

	if forkManagerInstance == nil {
		forkManagerInstance = &forkManager{}
		forkManagerInstance.Clear()
	}

	return forkManagerInstance
}

func (fm *forkManager) Clear() {
	fm.lock.Lock()
	defer fm.lock.Unlock()

	fm.forkMap = map[string]*Fork{}
	fm.handlersMap = map[HandlerDesc][]forkHandler{}
}

// RegisterFork registers fork by its name
func (fm *forkManager) RegisterFork(name string, forkParams *ForkParams) {
	fm.lock.Lock()
	defer fm.lock.Unlock()

	fm.forkMap[name] = &Fork{
		Name:            name,
		FromBlockNumber: 0,
		IsActive:        false,
		Params:          forkParams,
		Handlers:        map[HandlerDesc]interface{}{},
	}
}

// RegisterHandler registers handler by its name for specific fork
func (fm *forkManager) RegisterHandler(forkName string, handlerName HandlerDesc, handler interface{}) error {
	fm.lock.Lock()
	defer fm.lock.Unlock()

	fork, exists := fm.forkMap[forkName]
	if !exists {
		return fmt.Errorf("fork does not exist: %s", forkName)
	}

	fork.Handlers[handlerName] = handler

	return nil
}

// ActivateFork activates fork from some block number
// All handlers and parameters belonging to this fork are also activated
func (fm *forkManager) ActivateFork(forkName string, blockNumber uint64) error {
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

	for name, handler := range fork.Handlers {
		fm.addHandler(name, blockNumber, handler)
	}

	fm.addParams(blockNumber, fork.Params)

	return nil
}

// DeactivateFork de-activates fork
// All handlers and parameters belong to this fork are also de-activated
func (fm *forkManager) DeactivateFork(forkName string) error {
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

	fm.removeParams(fork.FromBlockNumber)

	return nil
}

// GetHandler retrieves handler for handler name and for a block number
func (fm *forkManager) GetHandler(name HandlerDesc, blockNumber uint64) interface{} {
	fm.lock.Lock()
	defer fm.lock.Unlock()

	handlers, exists := fm.handlersMap[name]
	if !exists {
		return nil
	}

	// binary search to find the latest handler defined for a specific block
	pos := sort.Search(len(handlers), func(i int) bool {
		return handlers[i].FromBlockNumber > blockNumber
	}) - 1
	if pos < 0 {
		return nil
	}

	return handlers[pos].Handler
}

// GetParams retrieves chain.ForkParams for a block number
func (fm *forkManager) GetParams(blockNumber uint64) *ForkParams {
	fm.lock.Lock()
	defer fm.lock.Unlock()

	// binary search to find the desired *chain.ForkParams
	pos := sort.Search(len(fm.params), func(i int) bool {
		return fm.params[i].FromBlockNumber > blockNumber
	}) - 1
	if pos < 0 {
		return nil
	}

	return fm.params[pos].Params
}

// IsForkRegistered checks if fork is registered
func (fm *forkManager) IsForkRegistered(name string) bool {
	fm.lock.Lock()
	defer fm.lock.Unlock()

	_, exists := fm.forkMap[name]

	return exists
}

// IsForkEnabled checks if fork is registered and enabled for specific block
func (fm *forkManager) IsForkEnabled(name string, blockNumber uint64) bool {
	fm.lock.Lock()
	defer fm.lock.Unlock()

	fork, exists := fm.forkMap[name]
	if !exists {
		return false
	}

	return fork.IsActive && fork.FromBlockNumber <= blockNumber
}

// GetForkBlock returns fork block if fork is registered and activated
func (fm *forkManager) GetForkBlock(name string) (uint64, error) {
	fm.lock.Lock()
	defer fm.lock.Unlock()

	fork, exists := fm.forkMap[name]
	if !exists {
		return 0, fmt.Errorf("fork does not exist: %s", name)
	}

	if !fork.IsActive {
		return 0, fmt.Errorf("fork is not active: %s", name)
	}

	return fork.FromBlockNumber, nil
}

func (fm *forkManager) addHandler(handlerName HandlerDesc, blockNumber uint64, handler interface{}) {
	if handlers, exists := fm.handlersMap[handlerName]; !exists {
		fm.handlersMap[handlerName] = []forkHandler{
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
		// replace existing handler if on the same block as current one
		if index < len(handlers) && handlers[index].FromBlockNumber == blockNumber {
			handlers[index].Handler = handler

			return
		}

		handlers = append(handlers, forkHandler{})
		copy(handlers[index+1:], handlers[index:])
		handlers[index] = forkHandler{
			FromBlockNumber: blockNumber,
			Handler:         handler,
		}
		fm.handlersMap[handlerName] = handlers
	}
}

func (fm *forkManager) removeHandler(handlerName HandlerDesc, blockNumber uint64) {
	handlers, exists := fm.handlersMap[handlerName]
	if !exists {
		return
	}

	index := sort.Search(len(handlers), func(i int) bool {
		return handlers[i].FromBlockNumber >= blockNumber
	})

	if index < len(handlers) && handlers[index].FromBlockNumber == blockNumber {
		copy(handlers[index:], handlers[index+1:])
		handlers[len(handlers)-1] = forkHandler{}
		fm.handlersMap[handlerName] = handlers[:len(handlers)-1]
	}
}

func (fm *forkManager) addParams(blockNumber uint64, params *ForkParams) {
	if params == nil {
		return
	}

	item := forkParamsBlock{FromBlockNumber: blockNumber, Params: params}

	if len(fm.params) == 0 {
		fm.params = append(fm.params, item)
	} else {
		// keep everything in sorted order
		index := sort.Search(len(fm.params), func(i int) bool {
			return fm.params[i].FromBlockNumber >= blockNumber
		})

		fm.params = append(fm.params, forkParamsBlock{})
		copy(fm.params[index+1:], fm.params[index:])
		fm.params[index] = item

		if index > 0 {
			// copy all nil parameters from previous
			copyParams(item.Params, fm.params[index-1].Params)
		}

		// update parameters for next
		for i := index; i < len(fm.params)-1; i++ {
			copyParams(fm.params[i+1].Params, fm.params[i].Params)
		}
	}
}

func (fm *forkManager) removeParams(blockNumber uint64) {
	index := sort.Search(len(fm.params), func(i int) bool {
		return fm.params[i].FromBlockNumber >= blockNumber
	})

	if index < len(fm.params) && fm.params[index].FromBlockNumber == blockNumber {
		copy(fm.params[index:], fm.params[index+1:])
		fm.params[len(fm.params)-1] = forkParamsBlock{}
		fm.params = fm.params[:len(fm.params)-1]
	}
}

func copyParams(dest, src *ForkParams) {
	srcValue := reflect.ValueOf(src).Elem()
	dstValue := reflect.ValueOf(dest).Elem()

	for i := 0; i < srcValue.NumField(); i++ {
		dstField := dstValue.Field(i)
		srcField := srcValue.Field(i)

		// copy if dst is nil, but src is not
		if dstField.Kind() == reflect.Ptr && dstField.IsNil() && !srcField.IsNil() {
			dstField.Set(srcField)
		}
	}
}
