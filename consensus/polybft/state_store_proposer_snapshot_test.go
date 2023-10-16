package polybft

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestState_getProposerSnapshot_writeProposerSnapshot(t *testing.T) {
	t.Parallel()

	const (
		height = uint64(100)
		round  = uint64(5)
	)

	state := newTestState(t)

	snap, err := state.ProposerSnapshotStore.getProposerSnapshot(nil)
	require.NoError(t, err)
	require.Nil(t, snap)

	newSnapshot := &ProposerSnapshot{Height: height, Round: round}
	require.NoError(t, state.ProposerSnapshotStore.writeProposerSnapshot(newSnapshot, nil))

	snap, err = state.ProposerSnapshotStore.getProposerSnapshot(nil)
	require.NoError(t, err)
	require.Equal(t, newSnapshot, snap)
}
