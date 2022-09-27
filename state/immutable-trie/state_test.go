package itrie

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/evm"
)

func TestState(t *testing.T) {
	evm.TestState(t, buildPreState)
}

func buildPreState(pre evm.PreStates) (evm.State, evm.Snapshot) {
	storage := NewMemoryStorage()
	st := NewState(storage)
	snap := st.NewSnapshot()

	return st, snap
}
