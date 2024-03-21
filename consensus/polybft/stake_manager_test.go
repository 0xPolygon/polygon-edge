package polybft

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/crypto"
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
		stakeManagerAddr  = types.StringToAddress("0x0001")
	)

	t.Run("PostBlock - unstake to zero", func(t *testing.T) {
		t.Parallel()

		state := newTestState(t)

		bcMock := new(blockchainMock)
		bcMock.On("CurrentHeader").Return(&types.Header{Number: block - 1}, true).Once()
		bcMock.On("GetStateProviderForBlock", mock.Anything).Return(nil).Times(len(allAliases))

		validators := validator.NewTestValidatorsWithAliases(t, allAliases)

		// insert initial full validator set
		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators:  newValidatorStakeMap(validators.GetPublicIdentities(initialSetAliases...)),
			BlockNumber: block - 1,
		}, nil))

		stakeManager, err := newStakeManager(
			hclog.NewNullLogger(),
			state,
			stakeManagerAddr,
			bcMock,
			nil,
			nil,
		)
		require.NoError(t, err)

		header := &types.Header{Number: block}

		require.NoError(t, stakeManager.ProcessLog(header, convertLog(createTestLogForStakeRemovedEvent(
			t,
			stakeManagerAddr,
			validators.GetValidator(initialSetAliases[firstValidator]).Address(),
			1, // initial validator stake was 1
		)), nil))

		req := &PostBlockRequest{
			FullBlock: &types.FullBlock{Block: &types.Block{Header: header}},
			Epoch:     epoch,
		}

		require.NoError(t, stakeManager.PostBlock(req))

		fullValidatorSet, err := state.StakeStore.getFullValidatorSet(nil)
		require.NoError(t, err)

		var firstValidatorMeta *validator.ValidatorMetadata

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

		state := newTestState(t)

		bcMock := new(blockchainMock)
		bcMock.On("CurrentHeader").Return(&types.Header{Number: block - 1}, true).Once()

		validators := validator.NewTestValidatorsWithAliases(t, allAliases)

		// insert initial full validator set
		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators:  newValidatorStakeMap(validators.GetPublicIdentities(initialSetAliases...)),
			BlockNumber: block - 1,
		}, nil))

		stakeManager, err := newStakeManager(
			hclog.NewNullLogger(),
			state,
			types.StringToAddress("0x0001"),
			bcMock,
			nil,
			nil,
		)
		require.NoError(t, err)

		header := &types.Header{Number: block}
		require.NoError(t, stakeManager.ProcessLog(header, convertLog(createTestLogForStakeAddedEvent(
			t,
			stakeManagerAddr,
			validators.GetValidator(initialSetAliases[secondValidator]).Address(),
			250,
		)), nil))

		req := &PostBlockRequest{
			FullBlock: &types.FullBlock{Block: &types.Block{Header: header}},
			Epoch:     epoch,
		}

		require.NoError(t, stakeManager.PostBlock(req))

		fullValidatorSet, err := state.StakeStore.getFullValidatorSet(nil)
		require.NoError(t, err)

		var firstValidator *validator.ValidatorMetadata

		for _, validator := range fullValidatorSet.Validators {
			if validator.Address.String() == validators.GetValidator(initialSetAliases[secondValidator]).Address().String() {
				firstValidator = validator
			}
		}

		require.NotNil(t, firstValidator)
		require.Equal(t, big.NewInt(251), firstValidator.VotingPower) // 250 + initial 1
		require.True(t, firstValidator.IsActive)
	})

	t.Run("PostBlock - add validator and stake", func(t *testing.T) {
		t.Parallel()

		state := newTestState(t)
		validators := validator.NewTestValidatorsWithAliases(t, allAliases, []uint64{1, 2, 3, 4, 5, 6})

		txRelayerMock := newDummyStakeTxRelayer(t, func() *validator.ValidatorMetadata {
			return validators.GetValidator("F").ValidatorMetadata()
		})
		// just mock the call however, the dummy relayer should do its magic
		txRelayerMock.On("Call", mock.Anything, mock.Anything, mock.Anything).
			Return(nil, error(nil))

		bcMock := new(blockchainMock)
		bcMock.On("CurrentHeader").Return(&types.Header{Number: block - 1}, true)
		bcMock.On("GetStateProviderForBlock", mock.Anything).Return(nil).Times(len(allAliases))

		// insert initial full validator set
		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators:  newValidatorStakeMap(validators.GetPublicIdentities(initialSetAliases...)),
			BlockNumber: block - 1,
		}, nil))

		stakeManager, err := newStakeManager(
			hclog.NewNullLogger(),
			state,
			types.StringToAddress("0x0001"),
			bcMock,
			nil,
			nil,
		)
		require.NoError(t, err)

		header := &types.Header{Number: block}

		for i := 0; i < len(allAliases); i++ {
			require.NoError(t, stakeManager.ProcessLog(header, convertLog(createTestLogForStakeAddedEvent(
				t,
				stakeManagerAddr,
				validators.GetValidator(allAliases[i]).Address(),
				newStake,
			)), nil))
		}

		req := &PostBlockRequest{
			FullBlock: &types.FullBlock{Block: &types.Block{Header: header}},
			Epoch:     epoch,
		}

		require.NoError(t, stakeManager.PostBlock(req))

		fullValidatorSet, err := state.StakeStore.getFullValidatorSet(nil)
		require.NoError(t, err)
		require.Len(t, fullValidatorSet.Validators, len(allAliases))

		validatorsCount := validators.ToValidatorSet().Len()
		for i, v := range fullValidatorSet.Validators.getSorted(validatorsCount) {
			require.Equal(t, newStake+uint64(validatorsCount)-uint64(i)-1, v.VotingPower.Uint64())
		}
	})
}

func TestStakeManager_UpdateValidatorSet(t *testing.T) {
	var (
		aliases             = []string{"A", "B", "C", "D", "E"}
		stakes              = []uint64{10, 10, 10, 10, 10}
		epoch               = uint64(1)
		maxValidatorSetSize = uint64(10)
	)

	validators := validator.NewTestValidatorsWithAliases(t, aliases, stakes)
	state := newTestState(t)

	bcMock := new(blockchainMock)
	bcMock.On("CurrentHeader").Return(&types.Header{Number: 0}, true).Once()

	require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
		Validators: newValidatorStakeMap(validators.ToValidatorSet().Accounts()),
	}, nil))

	stakeManager, err := newStakeManager(
		hclog.NewNullLogger(),
		state,
		types.StringToAddress("0x0001"),
		bcMock,
		nil,
		nil,
	)
	require.NoError(t, err)

	t.Run("UpdateValidatorSet - only update", func(t *testing.T) {
		fullValidatorSet := validators.GetPublicIdentities().Copy()
		validatorToUpdate := fullValidatorSet[0]
		validatorToUpdate.VotingPower = big.NewInt(11)

		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators: newValidatorStakeMap(fullValidatorSet),
		}, nil))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch, maxValidatorSetSize,
			validators.GetPublicIdentities())
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
		}, nil))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+1, maxValidatorSetSize,
			validators.GetPublicIdentities())

		require.NoError(t, err)
		require.Len(t, updateDelta.Added, 0)
		require.Len(t, updateDelta.Updated, 0)
		require.Len(t, updateDelta.Removed, 1)
	})

	t.Run("UpdateValidatorSet - one new validator", func(t *testing.T) {
		addedValidator := validators.GetValidator("A")

		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators: newValidatorStakeMap(validators.GetPublicIdentities()),
		}, nil))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+2, maxValidatorSetSize,
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
		}, nil))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+3, maxValidatorSetSize,
			validators.GetPublicIdentities())

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
		}, nil))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+4, maxValidatorSetSize,
			validators.GetPublicIdentities())

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
		}, nil))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+5, maxValidatorSetSize,
			validators.GetPublicIdentities())
		require.NoError(t, err)
		require.Len(t, updateDelta.Added, 0)
		require.Len(t, updateDelta.Updated, 0)
		require.Len(t, updateDelta.Removed, 1)
	})

	t.Run("UpdateValidatorSet - max validator set size reached", func(t *testing.T) {
		// because we now have 5 validators, and the new validator has more stake
		fullValidatorSet := validators.GetPublicIdentities().Copy()
		validatorToAdd := fullValidatorSet[0]
		validatorToAdd.VotingPower = big.NewInt(11)

		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators: newValidatorStakeMap(fullValidatorSet),
		}, nil))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+6, 4,
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

func TestStakeManager_UpdateOnInit(t *testing.T) {
	t.Parallel()

	var (
		allAliases       = []string{"A", "B", "C", "D", "E", "F"}
		stakeManagerAddr = types.StringToAddress("0xf001")
	)

	votingPowers := []uint64{1, 1, 1, 1, 5, 7}
	validators := validator.NewTestValidatorsWithAliases(t, allAliases, votingPowers)
	accountSet := validators.GetPublicIdentities(allAliases...)
	state := newTestState(t)

	polyBackendMock := new(polybftBackendMock)
	polyBackendMock.On("GetValidatorsWithTx", uint64(0), []*types.Header(nil), mock.Anything).Return(accountSet, nil).Once()

	_, err := newStakeManager(
		hclog.NewNullLogger(),
		state,
		stakeManagerAddr,
		nil,
		polyBackendMock,
		nil,
	)
	require.NoError(t, err)

	fullValidatorSet, err := state.StakeStore.getFullValidatorSet(nil)
	require.NoError(t, err)

	require.Equal(t, uint64(0), fullValidatorSet.BlockNumber)
	require.Equal(t, uint64(0), fullValidatorSet.UpdatedAtBlockNumber)
	require.Equal(t, uint64(0), fullValidatorSet.EpochID)

	for i, addr := range accountSet.GetAddresses() {
		v, exists := fullValidatorSet.Validators[addr]

		require.True(t, exists)
		require.Equal(t, big.NewInt(int64(votingPowers[i])), v.VotingPower)
	}
}

func createTestLogForStakeAddedEvent(t *testing.T, validatorSet, to types.Address, stake uint64) *types.Log {
	t.Helper()

	var stakeAddedEvent contractsapi.StakeAddedEvent

	topics := make([]types.Hash, 2)
	topics[0] = types.Hash(stakeAddedEvent.Sig())
	topics[1] = types.BytesToHash(to.Bytes())
	encodedData, err := abi.MustNewType("uint256").Encode(new(big.Int).SetUint64(stake))
	require.NoError(t, err)

	return &types.Log{
		Address: validatorSet,
		Topics:  topics,
		Data:    encodedData,
	}
}

func createTestLogForStakeRemovedEvent(t *testing.T, validatorSet, to types.Address, unstake uint64) *types.Log {
	t.Helper()

	var stakeRemovedEvent contractsapi.StakeRemovedEvent

	topics := make([]types.Hash, 2)
	topics[0] = types.Hash(stakeRemovedEvent.Sig())
	topics[1] = types.BytesToHash(to.Bytes())
	encodedData, err := abi.MustNewType("uint256").Encode(new(big.Int).SetUint64(unstake))
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

func (d *dummyStakeTxRelayer) Call(from types.Address, to types.Address, input []byte) (string, error) {
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

func (d *dummyStakeTxRelayer) SendTransaction(transaction *types.Transaction, key crypto.Key) (*ethgo.Receipt, error) {
	args := d.Called(transaction, key)

	return args.Get(0).(*ethgo.Receipt), args.Error(1)
}

// SendTransactionLocal sends non-signed transaction (this is only for testing purposes)
func (d *dummyStakeTxRelayer) SendTransactionLocal(txn *types.Transaction) (*ethgo.Receipt, error) {
	args := d.Called(txn)

	return args.Get(0).(*ethgo.Receipt), args.Error(1)
}

func (d *dummyStakeTxRelayer) Client() *jsonrpc.Client {
	return nil
}
