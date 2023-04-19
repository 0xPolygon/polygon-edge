package state

import (
	statetransition "github.com/0xPolygon/polygon-edge/state_transition"
	"github.com/0xPolygon/polygon-edge/types"
)

type State interface {
	NewSnapshotAt(types.Hash) (Snapshot, error)
	NewSnapshot() Snapshot
	GetCode(hash types.Hash) ([]byte, bool)
}

type Snapshot interface {
	statetransition.ReadSnapshot

	Commit(objs []*statetransition.Object) (Snapshot, []byte)
}
