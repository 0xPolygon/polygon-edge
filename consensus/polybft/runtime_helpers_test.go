package polybft

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestHelpers_isEpochEndingBlock_DeltaNotEmpty(t *testing.T) {
	t.Parallel()

	validators := validator.NewTestValidators(t, 3).GetPublicIdentities()
	bitmap := bitmap.Bitmap{}
	bitmap.Set(0)

	delta := &validator.ValidatorSetDelta{
		Added:   validators[1:],
		Removed: bitmap,
	}

	extra := &Extra{Validators: delta, Checkpoint: &CheckpointData{EpochNumber: 2}}
	blockNumber := uint64(20)

	isEndOfEpoch, err := isEpochEndingBlock(blockNumber, extra, new(blockchainMock))
	require.NoError(t, err)
	require.True(t, isEndOfEpoch)
}

func TestHelpers_isEpochEndingBlock_NoBlock(t *testing.T) {
	t.Parallel()

	blockchainMock := new(blockchainMock)
	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(&types.Header{}, false)

	extra := &Extra{Checkpoint: &CheckpointData{EpochNumber: 2}, Validators: &validator.ValidatorSetDelta{}}
	blockNumber := uint64(20)

	isEndOfEpoch, err := isEpochEndingBlock(blockNumber, extra, blockchainMock)
	require.ErrorIs(t, blockchain.ErrNoBlock, err)
	require.False(t, isEndOfEpoch)
}

func TestHelpers_isEpochEndingBlock_EpochsNotTheSame(t *testing.T) {
	t.Parallel()

	blockchainMock := new(blockchainMock)

	nextBlockExtra := &Extra{Checkpoint: &CheckpointData{EpochNumber: 3}, Validators: &validator.ValidatorSetDelta{}}
	nextBlock := &types.Header{
		Number:    21,
		ExtraData: nextBlockExtra.MarshalRLPTo(nil),
	}

	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(nextBlock, true)

	extra := &Extra{Checkpoint: &CheckpointData{EpochNumber: 2}, Validators: &validator.ValidatorSetDelta{}}
	blockNumber := uint64(20)

	isEndOfEpoch, err := isEpochEndingBlock(blockNumber, extra, blockchainMock)
	require.NoError(t, err)
	require.True(t, isEndOfEpoch)
}

func TestHelpers_isEpochEndingBlock_EpochsAreTheSame(t *testing.T) {
	t.Parallel()

	blockchainMock := new(blockchainMock)

	nextBlockExtra := &Extra{Checkpoint: &CheckpointData{EpochNumber: 2}, Validators: &validator.ValidatorSetDelta{}}
	nextBlock := &types.Header{
		Number:    16,
		ExtraData: nextBlockExtra.MarshalRLPTo(nil),
	}

	blockchainMock.On("GetHeaderByNumber", mock.Anything).Return(nextBlock, true)

	extra := &Extra{Checkpoint: &CheckpointData{EpochNumber: 2}, Validators: &validator.ValidatorSetDelta{}}
	blockNumber := uint64(15)

	isEndOfEpoch, err := isEpochEndingBlock(blockNumber, extra, blockchainMock)
	require.NoError(t, err)
	require.False(t, isEndOfEpoch)
}
