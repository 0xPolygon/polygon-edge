package itrie

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/state"
	"go.uber.org/goleak"
)

func TestState(t *testing.T) {
	defer goleak.VerifyNone(t)
	state.TestState(t, buildPreState)
}

func buildPreState(pre state.PreStates) (state.State, state.Snapshot) {
	storage := NewMemoryStorage()
	st := NewState(storage)
	snap := st.NewSnapshot()

	return st, snap
}
