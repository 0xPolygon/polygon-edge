package polybft

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestState_Insert_And_Get_FullValidatorSet(t *testing.T) {
	state := newTestState(t)

	t.Run("No full validator set", func(t *testing.T) {
		_, err := state.StakeStore.getFullValidatorSet()

		require.ErrorIs(t, err, errNoFullValidatorSet)
	})

	t.Run("Insert validator set", func(t *testing.T) {
		validators := newTestValidators(t, 5).getPublicIdentities()
		err := state.StakeStore.insertFullValidatorSet(validators)

		assert.NoError(t, err)

		fullValidatorSet, err := state.StakeStore.getFullValidatorSet()
		require.NoError(t, err)
		assert.Len(t, fullValidatorSet, len(validators))
	})

	t.Run("Update validator set", func(t *testing.T) {
		validators := newTestValidators(t, 10).getPublicIdentities()
		err := state.StakeStore.insertFullValidatorSet(validators)

		assert.NoError(t, err)

		fullValidatorSet, err := state.StakeStore.getFullValidatorSet()
		require.NoError(t, err)
		assert.Len(t, fullValidatorSet, len(validators))
	})
}
