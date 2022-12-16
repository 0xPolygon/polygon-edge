package polybft

import (
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

func newTestState(t *testing.T) *State {
	t.Helper()

	dir := fmt.Sprintf("/tmp/consensus-temp_%v", time.Now().Format(time.RFC3339Nano))
	err := os.Mkdir(dir, 0777)

	if err != nil {
		t.Fatal(err)
	}

	state, err := newState(path.Join(dir, "my.db"), hclog.NewNullLogger(), make(chan struct{}))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
	})

	return state
}

func TestState_getProposerSnapshot_writeProposerSnapshot(t *testing.T) {
	t.Parallel()

	const (
		height = uint64(100)
		round  = uint64(5)
	)

	state := newTestState(t)

	snap, err := state.getProposerSnapshot()
	require.NoError(t, err)
	require.Nil(t, snap)

	newSnapshot := &ProposerSnapshot{Height: height, Round: round}
	require.NoError(t, state.writeProposerSnapshot(newSnapshot))

	snap, err = state.getProposerSnapshot()
	require.NoError(t, err)
	require.Equal(t, newSnapshot, snap)
}
