package polybft

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	go_fuzz_utils "github.com/trailofbits/go-fuzz-utils"
)

func FuzzTestStakeManagerPostEpoch(f *testing.F) {
	state := newTestState(f)

	seeds := []struct {
		EpochID    uint64
		Validators AccountSet
	}{
		{
			EpochID:    0,
			Validators: newTestValidators(f, 6).getPublicIdentities(),
		},
		{
			EpochID:    1,
			Validators: newTestValidators(f, 42).getPublicIdentities(),
		},
		{
			EpochID:    42,
			Validators: newTestValidators(f, 6).getPublicIdentities(),
		},
	}

	for _, seed := range seeds {
		data, _ := json.Marshal(seed)
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		stakeManager := &stakeManager{
			logger:              hclog.NewNullLogger(),
			state:               state,
			maxValidatorSetSize: 10,
		}

		tp, err := go_fuzz_utils.NewTypeProvider(input)
		if err != nil {
			return
		}
		newEpochID, err := tp.GetUint64()
		if err != nil {
			return
		}
		var validators AccountSet
		err = tp.Fill(validators)
		if err != nil {
			return
		}

		_ = stakeManager.PostEpoch(&PostEpochRequest{
			NewEpochID: newEpochID,
			ValidatorSet: NewValidatorSet(
				validators,
				stakeManager.logger,
			),
		})
	})
}

func FuzzTestStakeManagerPostBlock(f *testing.F) {
	var (
		allAliases        = []string{"A", "B", "C", "D", "E", "F"}
		initialSetAliases = []string{"A", "B", "C", "D", "E"}
		validators        = newTestValidatorsWithAliases(f, allAliases)
		state             = newTestState(f)
	)

	f.Fuzz(func(t *testing.T, input []byte) {
		t.Parallel()

		tp, err := go_fuzz_utils.NewTypeProvider(input)
		if err != nil {
			return
		}

		stakeValue, err := tp.GetUint64()
		if err != nil {
			return
		}

		epoch, err := tp.GetUint64()
		if err != nil {
			return
		}
		validatorID, err := tp.GetUint64()
		if err != nil {
			return
		}

		if validatorID > uint64(len(initialSetAliases)-1) {
			t.Skip()
		}

		block, err := tp.GetUint64()
		if err != nil {
			return
		}

		systemStateMock := new(systemStateMock)
		systemStateMock.On("GetStakeOnValidatorSet", mock.Anything).Return(big.NewInt(int64(stakeValue)), nil).Once()

		blockchainMock := new(blockchainMock)
		blockchainMock.On("GetStateProviderForBlock", mock.Anything).Return(new(stateProviderMock)).Once()
		blockchainMock.On("GetSystemState", mock.Anything, mock.Anything).Return(systemStateMock)

		stakeManager := newStakeManager(
			hclog.NewNullLogger(),
			state,
			blockchainMock,
			nil,
			wallet.NewEcdsaSigner(validators.getValidator("A").Key()),
			types.StringToAddress("0x0001"),
			types.StringToAddress("0x0002"),
			5,
		)

		// insert initial full validator set
		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators: newValidatorStakeMap(validators.getPublicIdentities(initialSetAliases...)),
		}))

		receipt := &types.Receipt{
			Logs: []*types.Log{
				createTestLogForTransferEvent(
					t,
					stakeManager.validatorSetContract,
					validators.getValidator(initialSetAliases[validatorID]).Address(),
					types.ZeroAddress,
					stakeValue,
				),
			},
		}

		req := &PostBlockRequest{
			FullBlock: &types.FullBlock{Block: &types.Block{Header: &types.Header{Number: block}},
				Receipts: []*types.Receipt{receipt},
			},
			Epoch: epoch,
		}
		_ = stakeManager.PostBlock(req)
	})
}

func FuzzTestStakeManagerUpdateValidatorSet(f *testing.F) {
	var (
		aliases = []string{"A", "B", "C", "D", "E"}
		stakes  = []uint64{10, 10, 10, 10, 10}
	)

	validators := newTestValidatorsWithAliases(f, aliases, stakes)
	state := newTestState(f)

	stakeManager := newStakeManager(
		hclog.NewNullLogger(),
		state,
		nil,
		nil,
		wallet.NewEcdsaSigner(validators.getValidator("A").Key()),
		types.StringToAddress("0x0001"), types.StringToAddress("0x0002"),
		10,
	)

	f.Fuzz(func(t *testing.T, input []byte) {
		tp, err := go_fuzz_utils.NewTypeProvider(input)
		if err != nil {
			return
		}
		epoch, err := tp.GetUint64()
		if err != nil {
			return
		}
		x, err := tp.GetUint64()
		if err != nil {
			return
		}
		if x > uint64(len(aliases)-1) {
			t.Skip()
		}
		votingPower, err := tp.GetInt64()
		if err != nil {
			return
		}

		_, _ = stakeManager.UpdateValidatorSet(epoch, validators.getPublicIdentities(aliases[x:]...))

		fullValidatorSet := validators.getPublicIdentities().Copy()
		validatorToUpdate := fullValidatorSet[x]
		validatorToUpdate.VotingPower = big.NewInt(votingPower)

		_, _ = stakeManager.UpdateValidatorSet(epoch, validators.getPublicIdentities())
	})
}
