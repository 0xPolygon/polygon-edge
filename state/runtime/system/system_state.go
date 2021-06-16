package system

import (
	"sync"

	"github.com/0xPolygon/minimal/state/runtime"
)

// systemState defines the structure that holds the host and contract
type systemState struct {
	host     runtime.Host      // The runtime.Host is needed because of access to world state
	contract *runtime.Contract // The runtime.Contract is needed for access to method call arguments
}

// systemStatePool is a pool that handles synchronicity with systemState objects
var systemStatePool = sync.Pool{
	New: func() interface{} {
		return new(systemState)
	},
}

// acquireSystemState returns a new instance of the systemState from the pool
func acquireSystemState() *systemState {
	return systemStatePool.Get().(*systemState)
}

// releaseSystemState resets the passed in state, and returns it to the pool
func releaseSystemState(s *systemState) {
	s.reset()
	systemStatePool.Put(s)
}

// reset resets the systemState fields
func (s *systemState) reset() {
	s.contract = nil
	s.host = nil
}
