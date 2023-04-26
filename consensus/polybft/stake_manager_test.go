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

func TestStakeManager_PostBlock(t *testing.T) {
	var (
		aliases    = []string{"A", "B", "C", "D", "E"}
		epoch      = uint64(1)
		block      = uint64(10)
		stakeAdded = uint64(10)
	)

	validators := newTestValidatorsWithAliases(t, aliases)
	state := newTestState(t)

	stakeManager := newStakeManager(
		hclog.NewNullLogger(),
		state,
		nil,
		wallet.NewEcdsaSigner(validators.getValidator("A").Key()),
		types.StringToAddress("0x0001"), types.StringToAddress("0x0002"),
		5,
	)

	receipts := make([]*types.Receipt, len(aliases))
	for i := 0; i < len(aliases); i++ {
		receipts[i] = &types.Receipt{Logs: []*types.Log{
			createTestLogForTransferEvent(
				t,
				stakeManager.validatorSetContract,
				types.ZeroAddress,
				validators.getValidator(aliases[i]).Address(),
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

	t.Run("PostBlock - not epoch ending block", func(t *testing.T) {
		req.IsEpochEndingBlock = false
		require.NoError(t, stakeManager.PostBlock(req))

		events, err := state.StakeStore.getTransferEvents(epoch)
		require.NoError(t, err)
		require.Len(t, events, len(aliases))
	})

	t.Run("PostBlock - epoch ending block (transfer events are saved to the next epoch)", func(t *testing.T) {
		req.IsEpochEndingBlock = true
		require.NoError(t, stakeManager.PostBlock(req))

		events, err := state.StakeStore.getTransferEvents(epoch + 1)

		require.NoError(t, err)
		require.Len(t, events, len(aliases))
	})
}

func TestStakeManager_UpdateValidatorSet(t *testing.T) {
	var (
		aliases    = []string{"A", "B", "C", "D", "E"}
		stakes     = []uint64{10, 10, 10, 10, 10}
		epoch      = uint64(1)
		stakeAdded = uint64(1)
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
		events := make([]*contractsapi.TransferEvent, 0)
		for _, v := range validators.getPublicIdentities() {
			events = append(events, &contractsapi.TransferEvent{
				From:  types.ZeroAddress,
				To:    v.Address,
				Value: new(big.Int).SetUint64(stakeAdded),
			})
		}

		require.NoError(t, state.StakeStore.insertTransferEvents(epoch, events))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch, validators.getPublicIdentities())
		require.NoError(t, err)
		require.Len(t, updateDelta.Added, 0)
		require.Len(t, updateDelta.Updated, len(aliases))
		require.Len(t, updateDelta.Removed, 0)

		for _, v := range validators.getPublicIdentities() {
			require.True(t, updateDelta.Updated.ContainsAddress(v.Address))
			require.Equal(t, updateDelta.Updated.GetValidatorMetadata(v.Address).VotingPower.Uint64(),
				v.VotingPower.Uint64()+stakeAdded)
		}
	})

	t.Run("UpdateValidatorSet - one unstake", func(t *testing.T) {
		validator := validators.getValidator("A")

		events := []*contractsapi.TransferEvent{
			{
				From:  validator.Address(),
				To:    types.ZeroAddress,
				Value: new(big.Int).SetUint64(validator.votingPower),
			},
		}

		require.NoError(t, state.StakeStore.insertTransferEvents(epoch+1, events))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+1, validators.getPublicIdentities())
		require.NoError(t, err)
		require.Len(t, updateDelta.Added, 0)
		require.Len(t, updateDelta.Updated, 0)
		require.Len(t, updateDelta.Removed, 1)
	})

	t.Run("UpdateValidatorSet - one new validator", func(t *testing.T) {
		validator := newTestValidatorsWithAliases(t, []string{"F"}, []uint64{10}).getValidator("F")

		events := []*contractsapi.TransferEvent{
			{
				From:  types.ZeroAddress,
				To:    validator.Address(),
				Value: new(big.Int).SetUint64(validator.votingPower),
			},
		}

		require.NoError(t, state.StakeStore.insertTransferEvents(epoch+2, events))

		txRelayerMock := newDummyStakeTxRelayer(t, func() *ValidatorMetadata {
			return validator.ValidatorMetadata()
		})

		stakeManager.rootChainRelayer = txRelayerMock

		// just mock the call however, the dummy relayer should do its magic
		txRelayerMock.On("Call", mock.Anything, mock.Anything, mock.Anything).
			Return(nil, error(nil))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+2, validators.getPublicIdentities())
		require.NoError(t, err)
		require.Len(t, updateDelta.Added, 1)
		require.Len(t, updateDelta.Updated, 0)
		require.Len(t, updateDelta.Removed, 0)
		require.Equal(t, validator.Address(), updateDelta.Added[0].Address)
		require.Equal(t, uint64(10), updateDelta.Added[0].VotingPower.Uint64())
	})

	t.Run("UpdateValidatorSet - max validator set size reached", func(t *testing.T) {
		validator := newTestValidatorsWithAliases(t, []string{"F"}, []uint64{100}).getValidator("F")

		events := []*contractsapi.TransferEvent{
			{
				From:  types.ZeroAddress,
				To:    validator.Address(),
				Value: new(big.Int).SetUint64(validator.votingPower),
			},
		}

		require.NoError(t, state.StakeStore.insertTransferEvents(epoch+3, events))

		txRelayerMock := newDummyStakeTxRelayer(t, func() *ValidatorMetadata {
			return validator.ValidatorMetadata()
		})

		stakeManager.rootChainRelayer = txRelayerMock
		// this will make one existing validator to be removed from validator set
		// because we now have 6 validators, and the new validator has more stake
		stakeManager.maxValidatorSetSize = 5

		// just mock the call however, the dummy relayer should do its magic
		txRelayerMock.On("Call", mock.Anything, mock.Anything, mock.Anything).
			Return(nil, error(nil))

		updateDelta, err := stakeManager.UpdateValidatorSet(epoch+3, validators.getPublicIdentities())
		require.NoError(t, err)
		require.Len(t, updateDelta.Added, 1)
		require.Len(t, updateDelta.Updated, 0)
		require.Len(t, updateDelta.Removed, 1)
		require.Equal(t, validator.Address(), updateDelta.Added[0].Address)
		require.Equal(t, uint64(100), updateDelta.Added[0].VotingPower.Uint64())
	})
}

func TestStakeCounter_ShouldBeDeterministic(t *testing.T) {
	t.Parallel()

	const timesToExecute = 250

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

		newAliases := []string{"H", "J", "K"}
		newValidators := newTestValidatorsWithAliases(t, newAliases, []uint64{10, 10, 10})

		test := func() ([]stakeInfo, []stakeInfo) {
			stakeCounter := newStakeCounter(validators.getPublicIdentities())

			for _, v := range newValidators.getPublicIdentities() {
				stakeCounter.addStake(v.Address, v.VotingPower)
			}

			return stakeCounter.getSortedMaxSlice(maxValidatorSetSize)
		}

		initialSlice, initialToRemove := test()

		// stake counter and stake map should always be deterministic
		for i := 0; i < timesToExecute; i++ {
			currentSlice, currentToRemove := test()

			require.Len(t, currentSlice, len(initialSlice))
			require.Len(t, currentToRemove, len(initialToRemove))

			for i, si := range currentSlice {
				initialSi := initialSlice[i]
				require.Equal(t, si.address, initialSi.address)
				require.Equal(t, si.stake.Uint64(), initialSi.stake.Uint64())
			}

			for i, si := range currentToRemove {
				initialSi := initialToRemove[i]
				require.Equal(t, si.address, initialSi.address)
				require.Equal(t, si.stake.Uint64(), initialSi.stake.Uint64())
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

var (
	_ txrelayer.TxRelayer = (*dummyStakeTxRelayer)(nil)
	//nolint:godox
	// TODO - @goran-ethernal change this once we remove old ChildValidatorSet
	// and its stubs, to use the new validator stub from the new contract
	validatorTypeABI = abi.MustNewType("tuple(uint256[4] blsKey, uint256 stake, bool isWhitelisted, bool isActive)")
)

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
