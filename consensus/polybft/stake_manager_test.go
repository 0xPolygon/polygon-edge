package polybft

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/jsonrpc"
)

func TestStakeManager_PostEpoch(t *testing.T) {
	validators := validator.NewTestValidators(t, 5).GetPublicIdentities()
	state := newTestState(t)

	stakeManager := &stakeManager{
		logger:              hclog.NewNullLogger(),
		state:               state,
		maxValidatorSetSize: 10,
	}

	t.Run("Not first epoch", func(t *testing.T) {
		require.NoError(t, stakeManager.PostEpoch(&PostEpochRequest{
			NewEpochID:   2,
			ValidatorSet: validator.NewValidatorSet(validators, stakeManager.logger),
		}))

		_, err := state.StakeStore.getFullValidatorSet()
		require.ErrorIs(t, errNoFullValidatorSet, err)
	})

	t.Run("First epoch", func(t *testing.T) {
		require.NoError(t, stakeManager.PostEpoch(&PostEpochRequest{
			NewEpochID:   1,
			ValidatorSet: validator.NewValidatorSet(validators, stakeManager.logger),
		}))

		fullValidatorSet, err := state.StakeStore.getFullValidatorSet()
		require.NoError(t, err)
		require.Len(t, fullValidatorSet.Validators, len(validators))
		require.Equal(t, uint64(0), fullValidatorSet.EpochID)
		require.Equal(t, uint64(0), fullValidatorSet.BlockNumber)
	})
}

func TestStakeManager_PostBlock(t *testing.T) {
	t.Parallel()

	var (
		allAliases        = []string{"A", "B", "C", "D", "E", "F"}
		initialSetAliases = []string{"A", "B", "C", "D", "E"}
		epoch             = uint64(1)
		block             = uint64(10)
		newStake          = uint64(100)
		firstValidator    = uint64(0)
		secondValidator   = uint64(1)
	)

	state := newTestState(t)
	t.Run("PostBlock - unstake to zero", func(t *testing.T) {
		t.Parallel()

		validators := validator.NewTestValidatorsWithAliases(t, allAliases)
		stakeManager := newStakeManager(
			hclog.NewNullLogger(),
			state,
			nil,
			wallet.NewEcdsaSigner(validators.GetValidator("A").Key()),
			types.StringToAddress("0x0001"), types.StringToAddress("0x0002"),
			nil,
			5,
		)

		// insert initial full validator set
		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators:  newValidatorStakeMap(validators.GetPublicIdentities(initialSetAliases...)),
			BlockNumber: block - 1,
		}))

		receipt := &types.Receipt{
			Logs: []*types.Log{
				createTestLogForTransferEvent(
					t,
					stakeManager.validatorSetContract,
					validators.GetValidator(initialSetAliases[firstValidator]).Address(),
					types.ZeroAddress,
					1, // initial validator stake was 1
				),
			},
		}

		receipt.SetStatus(types.ReceiptSuccess)

		req := &PostBlockRequest{
			FullBlock: &types.FullBlock{Block: &types.Block{Header: &types.Header{Number: block}},
				Receipts: []*types.Receipt{receipt},
			},
			Epoch: epoch,
		}

		require.NoError(t, stakeManager.PostBlock(req))

		fullValidatorSet, err := state.StakeStore.getFullValidatorSet()
		require.NoError(t, err)
		var firstValidatorMeta *validator.ValidatorMetadata
		firstValidatorMeta = nil
		for _, validator := range fullValidatorSet.Validators {
			if validator.Address.String() == validators.GetValidator(initialSetAliases[firstValidator]).Address().String() {
				firstValidatorMeta = validator
			}
		}
		require.NotNil(t, firstValidatorMeta)
		require.Equal(t, bigZero, firstValidatorMeta.VotingPower)
		require.False(t, firstValidatorMeta.IsActive)
	})
	t.Run("PostBlock - add stake to one validator", func(t *testing.T) {
		t.Parallel()

		validators := validator.NewTestValidatorsWithAliases(t, allAliases)
		stakeManager := newStakeManager(
			hclog.NewNullLogger(),
			state,
			nil,
			wallet.NewEcdsaSigner(validators.GetValidator("A").Key()),
			types.StringToAddress("0x0001"), types.StringToAddress("0x0002"),
			nil,
			5,
		)

		// insert initial full validator set
		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators:  newValidatorStakeMap(validators.GetPublicIdentities(initialSetAliases...)),
			BlockNumber: block - 1,
		}))

		receipt := &types.Receipt{
			Logs: []*types.Log{
				createTestLogForTransferEvent(
					t,
					stakeManager.validatorSetContract,
					types.ZeroAddress,
					validators.GetValidator(initialSetAliases[secondValidator]).Address(),
					250,
				),
			},
		}

		receipt.SetStatus(types.ReceiptSuccess)

		req := &PostBlockRequest{
			FullBlock: &types.FullBlock{Block: &types.Block{Header: &types.Header{Number: block}},
				Receipts: []*types.Receipt{receipt},
			},
			Epoch: epoch,
		}

		require.NoError(t, stakeManager.PostBlock(req))

		fullValidatorSet, err := state.StakeStore.getFullValidatorSet()
		require.NoError(t, err)
		var firstValidaotor *validator.ValidatorMetadata
		firstValidaotor = nil
		for _, validator := range fullValidatorSet.Validators {
			if validator.Address.String() == validators.GetValidator(initialSetAliases[secondValidator]).Address().String() {
				firstValidaotor = validator
			}
		}
		require.NotNil(t, firstValidaotor)
		require.Equal(t, big.NewInt(251), firstValidaotor.VotingPower) // 250 + initial 1
		require.True(t, firstValidaotor.IsActive)
	})

	t.Run("PostBlock - add validator and stake", func(t *testing.T) {
		t.Parallel()

		validators := validator.NewTestValidatorsWithAliases(t, allAliases, []uint64{1, 2, 3, 4, 5, 6})

		txRelayerMock := newDummyStakeTxRelayer(t, func() *validator.ValidatorMetadata {
			return validators.GetValidator("F").ValidatorMetadata()
		})

		// just mock the call however, the dummy relayer should do its magic
		txRelayerMock.On("Call", mock.Anything, mock.Anything, mock.Anything).
			Return(nil, error(nil))

		stakeManager := newStakeManager(
			hclog.NewNullLogger(),
			state,
			txRelayerMock,
			wallet.NewEcdsaSigner(validators.GetValidator("A").Key()),
			types.StringToAddress("0x0001"), types.StringToAddress("0x0002"),
			nil,
			5,
		)

		// insert initial full validator set
		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators:  newValidatorStakeMap(validators.GetPublicIdentities(initialSetAliases...)),
			BlockNumber: block - 1,
		}))

		receipts := make([]*types.Receipt, len(allAliases))
		for i := 0; i < len(allAliases); i++ {
			receipts[i] = &types.Receipt{Logs: []*types.Log{
				createTestLogForTransferEvent(
					t,
					stakeManager.validatorSetContract,
					types.ZeroAddress,
					validators.GetValidator(allAliases[i]).Address(),
					newStake,
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
		require.Len(t, fullValidatorSet.Validators, len(allAliases))

		validatorsCount := validators.ToValidatorSet().Len()
		for i, v := range fullValidatorSet.Validators.getSorted(validatorsCount) {
			require.Equal(t, newStake+uint64(validatorsCount)-uint64(i)-1, v.VotingPower.Uint64())
		}
	})

	t.Run("PostBlock - add stake to one validator + missing block", func(t *testing.T) {
		t.Parallel()

		receipt := &types.Receipt{}
		header1, header2 := &types.Header{Hash: types.Hash{3, 2}}, &types.Header{Hash: types.Hash{6, 4}}

		bcMock := new(blockchainMock)
		bcMock.On("GetHeaderByNumber", block-2).Return(header1, true).Once()
		bcMock.On("GetHeaderByNumber", block-1).Return(header2, true).Once()
		bcMock.On("GetReceiptsByHash", header1.Hash).Return([]*types.Receipt{receipt}, error(nil)).Once()
		bcMock.On("GetReceiptsByHash", header2.Hash).Return([]*types.Receipt{}, error(nil)).Once()

		validators := validator.NewTestValidatorsWithAliases(t, allAliases)
		stakeManager := newStakeManager(
			hclog.NewNullLogger(),
			state,
			nil,
			wallet.NewEcdsaSigner(validators.GetValidator("A").Key()),
			types.StringToAddress("0x0001"), types.StringToAddress("0x0002"),
			bcMock,
			5,
		)

		// insert initial full validator set
		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators:  newValidatorStakeMap(validators.GetPublicIdentities(initialSetAliases...)),
			BlockNumber: block - 3,
		}))

		receipt.Logs = []*types.Log{
			createTestLogForTransferEvent(
				t,
				stakeManager.validatorSetContract,
				types.ZeroAddress,
				validators.GetValidator(initialSetAliases[secondValidator]).Address(),
				250,
			),
		}
		receipt.SetStatus(types.ReceiptSuccess)

		req := &PostBlockRequest{
			FullBlock: &types.FullBlock{Block: &types.Block{Header: &types.Header{Number: block}},
				Receipts: []*types.Receipt{receipt},
			},
			Epoch: epoch,
		}

		require.NoError(t, stakeManager.PostBlock(req))

		fullValidatorSet, err := state.StakeStore.getFullValidatorSet()
		require.NoError(t, err)
		var firstValidaotor *validator.ValidatorMetadata
		firstValidaotor = nil
		for _, validator := range fullValidatorSet.Validators {
			if validator.Address.String() == validators.GetValidator(initialSetAliases[secondValidator]).Address().String() {
				firstValidaotor = validator
			}
		}
		require.NotNil(t, firstValidaotor)
		require.Equal(t, big.NewInt(501), firstValidaotor.VotingPower) // 250 + 250 + initial 1
		require.True(t, firstValidaotor.IsActive)

		bcMock.AssertExpectations(t)
	})
}

func TestStakeManager_UpdateValidatorSet(t *testing.T) {
	var (
		aliases = []string{"A", "B", "C", "D", "E"}
		stakes  = []uint64{10, 10, 10, 10, 10}
		epoch   = uint64(1)
	)

	validators := validator.NewTestValidatorsWithAliases(t, aliases, stakes)
	state := newTestState(t)

	stakeManager := newStakeManager(
		hclog.NewNullLogger(),
		state,
		nil,
		wallet.NewEcdsaSigner(validators.GetValidator("A").Key()),
		types.StringToAddress("0x0001"), types.StringToAddress("0x0002"),
		nil,
		10,
	)

	t.Run("UpdateValidatorSet - only update", func(t *testing.T) {
		fullValidatorSet := validators.GetPublicIdentities().Copy()
		validatorToUpdate := fullValidatorSet[0]
		validatorToUpdate.VotingPower = big.NewInt(11)

		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators: newValidatorStakeMap(fullValidatorSet),
		}))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch, validators.GetPublicIdentities())
		require.NoError(t, err)
		require.Len(t, updateDelta.Added, 0)
		require.Len(t, updateDelta.Updated, 1)
		require.Len(t, updateDelta.Removed, 0)
		require.Equal(t, updateDelta.Updated[0].Address, validatorToUpdate.Address)
		require.Equal(t, updateDelta.Updated[0].VotingPower.Uint64(), validatorToUpdate.VotingPower.Uint64())
	})

	t.Run("UpdateValidatorSet - one unstake", func(t *testing.T) {
		fullValidatorSet := validators.GetPublicIdentities(aliases[1:]...)

		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators: newValidatorStakeMap(fullValidatorSet),
		}))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+1, validators.GetPublicIdentities())
		require.NoError(t, err)
		require.Len(t, updateDelta.Added, 0)
		require.Len(t, updateDelta.Updated, 0)
		require.Len(t, updateDelta.Removed, 1)
	})

	t.Run("UpdateValidatorSet - one new validator", func(t *testing.T) {
		addedValidator := validators.GetValidator("A")

		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators: newValidatorStakeMap(validators.GetPublicIdentities()),
		}))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+2,
			validators.GetPublicIdentities(aliases[1:]...))
		require.NoError(t, err)
		require.Len(t, updateDelta.Added, 1)
		require.Len(t, updateDelta.Updated, 0)
		require.Len(t, updateDelta.Removed, 0)
		require.Equal(t, addedValidator.Address(), updateDelta.Added[0].Address)
		require.Equal(t, addedValidator.VotingPower, updateDelta.Added[0].VotingPower.Uint64())
	})
	t.Run("UpdateValidatorSet - remove some stake", func(t *testing.T) {
		fullValidatorSet := validators.GetPublicIdentities().Copy()
		validatorToUpdate := fullValidatorSet[2]
		validatorToUpdate.VotingPower = big.NewInt(5)
		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators: newValidatorStakeMap(fullValidatorSet),
		}))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+3, validators.GetPublicIdentities())
		require.NoError(t, err)
		require.Len(t, updateDelta.Added, 0)
		require.Len(t, updateDelta.Updated, 1)
		require.Len(t, updateDelta.Removed, 0)
		require.Equal(t, updateDelta.Updated[0].Address, validatorToUpdate.Address)
		require.Equal(t, updateDelta.Updated[0].VotingPower.Uint64(), validatorToUpdate.VotingPower.Uint64())
	})
	t.Run("UpdateValidatorSet - remove entire stake", func(t *testing.T) {
		fullValidatorSet := validators.GetPublicIdentities().Copy()
		validatorToUpdate := fullValidatorSet[3]
		validatorToUpdate.VotingPower = bigZero
		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators: newValidatorStakeMap(fullValidatorSet),
		}))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+4, validators.GetPublicIdentities())
		require.NoError(t, err)
		require.Len(t, updateDelta.Added, 0)
		require.Len(t, updateDelta.Updated, 0)
		require.Len(t, updateDelta.Removed, 1)
	})
	t.Run("UpdateValidatorSet - voting power negative", func(t *testing.T) {
		fullValidatorSet := validators.GetPublicIdentities().Copy()
		validatorsToUpdate := fullValidatorSet[4]
		validatorsToUpdate.VotingPower = bigZero
		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators: newValidatorStakeMap(fullValidatorSet),
		}))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+5, validators.GetPublicIdentities())
		require.NoError(t, err)
		require.Len(t, updateDelta.Added, 0)
		require.Len(t, updateDelta.Updated, 0)
		require.Len(t, updateDelta.Removed, 1)
	})

	t.Run("UpdateValidatorSet - max validator set size reached", func(t *testing.T) {
		// because we now have 5 validators, and the new validator has more stake
		stakeManager.maxValidatorSetSize = 4

		fullValidatorSet := validators.GetPublicIdentities().Copy()
		validatorToAdd := fullValidatorSet[0]
		validatorToAdd.VotingPower = big.NewInt(11)

		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators: newValidatorStakeMap(fullValidatorSet),
		}))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+6,
			validators.GetPublicIdentities(aliases[1:]...))

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
		validators := validator.NewTestValidatorsWithAliases(t, aliases, stake)

		test := func() []*validator.ValidatorMetadata {
			stakeCounter := newValidatorStakeMap(validators.GetPublicIdentities("A", "B", "C", "D", "E"))

			return stakeCounter.getSorted(maxValidatorSetSize)
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
	callback func() *validator.ValidatorMetadata
	t        *testing.T
}

func newDummyStakeTxRelayer(t *testing.T, callback func() *validator.ValidatorMetadata) *dummyStakeTxRelayer {
	t.Helper()

	return &dummyStakeTxRelayer{
		t:        t,
		callback: callback,
	}
}

func (d *dummyStakeTxRelayer) Call(from ethgo.Address, to ethgo.Address, input []byte) (string, error) {
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

func (d *dummyStakeTxRelayer) Client() *jsonrpc.Client {
	return nil
}
