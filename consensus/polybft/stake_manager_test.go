package polybft

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

func TestStakeManager_PostEpoch(t *testing.T) {
	validators := newTestValidators(t, 5).getPublicIdentities()
	state := newTestState(t)

	stakeManager := &stakeManager{
		logger:              hclog.NewNullLogger(),
		state:               state,
		maxValidatorSetSize: 10,
	}

	t.Run("Not first epoch", func(t *testing.T) {
		require.NoError(t, stakeManager.PostEpoch(&PostEpochRequest{
			NewEpochID:   2,
			ValidatorSet: NewValidatorSet(validators, stakeManager.logger),
		}))

		_, err := state.StakeStore.getFullValidatorSet()
		require.ErrorIs(t, errNoFullValidatorSet, err)
	})

	t.Run("First epoch", func(t *testing.T) {
		require.NoError(t, stakeManager.PostEpoch(&PostEpochRequest{
			NewEpochID:   1,
			ValidatorSet: NewValidatorSet(validators, stakeManager.logger),
		}))

		fullValidatorSet, err := state.StakeStore.getFullValidatorSet()
		require.NoError(t, err)
		require.Len(t, fullValidatorSet, len(validators))
	})
}

func TestStakeManager_PostBlock(t *testing.T) {
	t.Parallel()

	var (
		allAliases        = []string{"A", "B", "C", "D", "E", "F"}
		initialSetAliases = []string{"A", "B", "C", "D", "E"}
		epoch             = uint64(1)
		block             = uint64(10)
		stakeAdded        = uint64(10)
	)

	validators := newTestValidatorsWithAliases(t, allAliases)
	state := newTestState(t)

	txRelayerMock := newDummyStakeTxRelayer(t, func() *ValidatorMetadata {
		return validators.getValidator("F").ValidatorMetadata()
	})

	// just mock the call however, the dummy relayer should do its magic
	txRelayerMock.On("Call", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, error(nil))

	stakeManager := newStakeManager(
		hclog.NewNullLogger(),
		state,
		txRelayerMock,
		wallet.NewEcdsaSigner(validators.getValidator("A").Key()),
		types.StringToAddress("0x0001"), types.StringToAddress("0x0002"),
		5,
	)

	// insert initial full validator set
	require.NoError(t, state.StakeStore.insertFullValidatorSet(
		validators.getPublicIdentities(initialSetAliases...)))

	receipts := make([]*types.Receipt, len(allAliases))
	for i := 0; i < len(allAliases); i++ {
		receipts[i] = &types.Receipt{Logs: []*types.Log{
			createTestLogForTransferEvent(
				t,
				stakeManager.validatorSetContract,
				types.ZeroAddress,
				validators.getValidator(allAliases[i]).Address(),
				stakeAdded,
			),
		}}
		receipts[i].SetStatus(types.ReceiptSuccess)
	}

	req := &PostBlockRequest{
		FullBlock: &types.FullBlock{Block: &types.Block{Header: &types.Header{Number: block}},
			Receipts: receipts},
		Epoch: epoch,
	}

	require.NoError(t, stakeManager.PostBlock(req))

	fullValidatorSet, err := state.StakeStore.getFullValidatorSet()
	require.NoError(t, err)
	require.Len(t, fullValidatorSet, len(allAliases))
}

func TestStakeManager_UpdateValidatorSet(t *testing.T) {
	var (
		aliases = []string{"A", "B", "C", "D", "E"}
		stakes  = []uint64{10, 10, 10, 10, 10}
		epoch   = uint64(1)
	)

	validators := newTestValidatorsWithAliases(t, aliases, stakes)
	state := newTestState(t)

	stakeManager := newStakeManager(
		hclog.NewNullLogger(),
		state,
		nil,
		wallet.NewEcdsaSigner(validators.getValidator("A").Key()),
		types.StringToAddress("0x0001"), types.StringToAddress("0x0002"),
		10,
	)

	t.Run("UpdateValidatorSet - only update", func(t *testing.T) {
		fullValidatorSet := validators.getPublicIdentities().Copy()
		validatorToUpdate := fullValidatorSet[0]
		validatorToUpdate.VotingPower = big.NewInt(11)

		require.NoError(t, state.StakeStore.insertFullValidatorSet(fullValidatorSet))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch, validators.getPublicIdentities())
		require.NoError(t, err)
		require.Len(t, updateDelta.Added, 0)
		require.Len(t, updateDelta.Updated, 1)
		require.Len(t, updateDelta.Removed, 0)
		require.Equal(t, updateDelta.Updated[0].Address, validatorToUpdate.Address)
		require.Equal(t, updateDelta.Updated[0].VotingPower.Uint64(), validatorToUpdate.VotingPower.Uint64())
	})

	t.Run("UpdateValidatorSet - one unstake", func(t *testing.T) {
		fullValidatorSet := validators.getPublicIdentities(aliases[1:]...)

		require.NoError(t, state.StakeStore.insertFullValidatorSet(fullValidatorSet))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+1, validators.getPublicIdentities())
		require.NoError(t, err)
		require.Len(t, updateDelta.Added, 0)
		require.Len(t, updateDelta.Updated, 0)
		require.Len(t, updateDelta.Removed, 1)
	})

	t.Run("UpdateValidatorSet - one new validator", func(t *testing.T) {
		addedValidator := validators.getValidator("A")
		require.NoError(t, state.StakeStore.insertFullValidatorSet(validators.getPublicIdentities()))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+2,
			validators.getPublicIdentities(aliases[1:]...))
		require.NoError(t, err)
		require.Len(t, updateDelta.Added, 1)
		require.Len(t, updateDelta.Updated, 0)
		require.Len(t, updateDelta.Removed, 0)
		require.Equal(t, addedValidator.Address(), updateDelta.Added[0].Address)
		require.Equal(t, addedValidator.votingPower, updateDelta.Added[0].VotingPower.Uint64())
	})

	t.Run("UpdateValidatorSet - max validator set size reached", func(t *testing.T) {
		// because we now have 5 validators, and the new validator has more stake
		stakeManager.maxValidatorSetSize = 4

		fullValidatorSet := validators.getPublicIdentities().Copy()
		validatorToAdd := fullValidatorSet[0]
		validatorToAdd.VotingPower = big.NewInt(11)

		require.NoError(t, state.StakeStore.insertFullValidatorSet(fullValidatorSet))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+3,
			validators.getPublicIdentities(aliases[1:]...))

		require.NoError(t, err)
		require.Len(t, updateDelta.Added, 1)
		require.Len(t, updateDelta.Updated, 0)
		require.Len(t, updateDelta.Removed, 1)
		require.Equal(t, validatorToAdd.Address, updateDelta.Added[0].Address)
		require.Equal(t, validatorToAdd.VotingPower.Uint64(), updateDelta.Added[0].VotingPower.Uint64())
	})
}

func TestStakeCounter_ShouldBeDeterministic(t *testing.T) {
	t.Parallel()

	const timesToExecute = 100

	stakes := [][]uint64{
		{103, 102, 101, 51, 50, 30, 10},
		{100, 100, 100, 50, 50, 30, 10},
		{103, 102, 101, 51, 50, 30, 10},
		{100, 100, 100, 50, 50, 30, 10},
	}
	maxValidatorSetSizes := []int{1000, 1000, 5, 6}

	for ind, stake := range stakes {
		maxValidatorSetSize := maxValidatorSetSizes[ind]

		aliases := []string{"A", "B", "C", "D", "E", "F", "G"}
		validators := newTestValidatorsWithAliases(t, aliases, stake)

		test := func() []*ValidatorMetadata {
			stakeCounter := newValidatorStakeMap(validators.getPublicIdentities("A", "B", "C", "D", "E"))

			return stakeCounter.getActiveValidators(maxValidatorSetSize)
		}

		initialSlice := test()

		// stake counter and stake map should always be deterministic
		for i := 0; i < timesToExecute; i++ {
			currentSlice := test()

			require.Len(t, currentSlice, len(initialSlice))

			for i, si := range currentSlice {
				initialSi := initialSlice[i]
				require.Equal(t, si.Address, initialSi.Address)
				require.Equal(t, si.VotingPower.Uint64(), initialSi.VotingPower.Uint64())
			}
		}
	}
}

func createTestLogForTransferEvent(t *testing.T, validatorSet, from, to types.Address, stake uint64) *types.Log {
	t.Helper()

	var transferEvent contractsapi.TransferEvent

	topics := make([]types.Hash, 3)
	topics[0] = types.Hash(transferEvent.Sig())
	topics[1] = types.BytesToHash(from.Bytes())
	topics[2] = types.BytesToHash(to.Bytes())
	encodedData, err := abi.MustNewType("uint256").Encode(new(big.Int).SetUint64(stake))
	require.NoError(t, err)

	return &types.Log{
		Address: validatorSet,
		Topics:  topics,
		Data:    encodedData,
	}
}

var _ txrelayer.TxRelayer = (*dummyStakeTxRelayer)(nil)

type dummyStakeTxRelayer struct {
	mock.Mock
	callback func() *ValidatorMetadata
	t        *testing.T
}

func newDummyStakeTxRelayer(t *testing.T, callback func() *ValidatorMetadata) *dummyStakeTxRelayer {
	t.Helper()

	return &dummyStakeTxRelayer{
		t:        t,
		callback: callback,
	}
}

func (d dummyStakeTxRelayer) Call(from ethgo.Address, to ethgo.Address, input []byte) (string, error) {
	args := d.Called(from, to, input)

	if d.callback != nil {
		validatorMetaData := d.callback()
		encoded, err := validatorTypeABI.Encode(map[string]interface{}{
			"blsKey":        validatorMetaData.BlsKey.ToBigInt(),
			"stake":         validatorMetaData.VotingPower,
			"isWhitelisted": true,
			"isActive":      true,
		})

		require.NoError(d.t, err)

		return hex.EncodeToHex(encoded), nil
	}

	return args.String(0), args.Error(1)
}

func (d *dummyStakeTxRelayer) SendTransaction(transaction *ethgo.Transaction, key ethgo.Key) (*ethgo.Receipt, error) {
	args := d.Called(transaction, key)

	return args.Get(0).(*ethgo.Receipt), args.Error(1) //nolint:forcetypeassert
}

// SendTransactionLocal sends non-signed transaction (this is only for testing purposes)
func (d *dummyStakeTxRelayer) SendTransactionLocal(txn *ethgo.Transaction) (*ethgo.Receipt, error) {
	args := d.Called(txn)

	return args.Get(0).(*ethgo.Receipt), args.Error(1) //nolint:forcetypeassert
}

func (d *dummyStakeTxRelayer) GetGasPrice() (uint64, error) {
	return 0, nil
}
