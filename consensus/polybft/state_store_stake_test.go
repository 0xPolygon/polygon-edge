package polybft

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestState_Insert_And_Get_FullValidatorSet(t *testing.T) {
	state := newTestState(t)

	t.Run("No full validator set", func(t *testing.T) {
		_, err := state.StakeStore.getFullValidatorSet(nil)

		require.ErrorIs(t, err, errNoFullValidatorSet)
	})

	t.Run("Insert validator set", func(t *testing.T) {
		validators := validator.NewTestValidators(t, 5).GetPublicIdentities()

		assert.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			BlockNumber: 100,
			EpochID:     10,
			Validators:  newValidatorStakeMap(validators),
		}, nil))

		fullValidatorSet, err := state.StakeStore.getFullValidatorSet(nil)
		require.NoError(t, err)
		assert.Equal(t, uint64(100), fullValidatorSet.BlockNumber)
		assert.Equal(t, uint64(10), fullValidatorSet.EpochID)
		assert.Len(t, fullValidatorSet.Validators, len(validators))
	})

	t.Run("Update validator set", func(t *testing.T) {
		validators := validator.NewTestValidators(t, 10).GetPublicIdentities()

		assert.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			BlockNumber: 40,
			EpochID:     4,
			Validators:  newValidatorStakeMap(validators),
		}, nil))

		fullValidatorSet, err := state.StakeStore.getFullValidatorSet(nil)
		require.NoError(t, err)
		assert.Len(t, fullValidatorSet.Validators, len(validators))
		assert.Equal(t, uint64(40), fullValidatorSet.BlockNumber)
		assert.Equal(t, uint64(4), fullValidatorSet.EpochID)
	})
}
