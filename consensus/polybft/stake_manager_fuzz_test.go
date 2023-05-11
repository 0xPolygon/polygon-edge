package polybft

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path"
	"strconv"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	go_fuzz_utils "github.com/trailofbits/go-fuzz-utils"
)

func FuzzTestStakeManager_PostEpoch(f *testing.F) {
	state := newTestStateF(f)

	seeds := []struct {
		EpochId    uint64
		Validators AccountSet
	}{
		{
			EpochId:    0,
			Validators: newTestValidatorsF(f, 6).getPublicIdentities(),
		},
		{
			EpochId:    1,
			Validators: newTestValidatorsF(f, 42).getPublicIdentities(),
		},
		{
			EpochId:    42,
			Validators: newTestValidatorsF(f, 6).getPublicIdentities(),
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
		newEpochId, err := tp.GetUint64()
		if err != nil {
			return
		}
		var validators AccountSet
		err = tp.Fill(validators)
		if err != nil {
			return
		}

		stakeManager.PostEpoch(&PostEpochRequest{
			NewEpochID: newEpochId,
			ValidatorSet: NewValidatorSet(
				validators,
				stakeManager.logger,
			),
		})
	})
}

func FuzzTestStakeManager_PostBlock(f *testing.F) {
	var (
		allAliases        = []string{"A", "B", "C", "D", "E", "F"}
		initialSetAliases = []string{"A", "B", "C", "D", "E"}
	)
	validators := newTestValidatorsWithAliasesF(f, allAliases)
	state := newTestStateF(f)

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
		validatorId, err := tp.GetUint64()
		if err != nil {
			return
		}

		if validatorId > uint64(len(initialSetAliases)-1) {
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
					validators.getValidator(initialSetAliases[validatorId]).Address(),
					types.ZeroAddress,
					uint64(stakeValue),
				),
			},
		}

		req := &PostBlockRequest{
			FullBlock: &types.FullBlock{Block: &types.Block{Header: &types.Header{Number: block}},
				Receipts: []*types.Receipt{receipt},
			},
			Epoch: epoch,
		}
		stakeManager.PostBlock(req)
	})
}

func FuzzTestStakeManager_UpdateValidatorSet(f *testing.F) {
	var (
		aliases = []string{"A", "B", "C", "D", "E"}
		stakes  = []uint64{10, 10, 10, 10, 10}
	)

	validators := newTestValidatorsWithAliasesF(f, aliases, stakes)
	state := newTestStateF(f)

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

		stakeManager.UpdateValidatorSet(epoch, validators.getPublicIdentities(aliases[x:]...))

		fullValidatorSet := validators.getPublicIdentities().Copy()
		validatorToUpdate := fullValidatorSet[x]
		validatorToUpdate.VotingPower = big.NewInt(votingPower)

		stakeManager.UpdateValidatorSet(epoch, validators.getPublicIdentities())

	})
}

func newTestValidatorsF(f *testing.F, validatorsCount int) *testValidators {
	f.Helper()

	aliases := make([]string, validatorsCount)
	for i := 0; i < validatorsCount; i++ {
		aliases[i] = strconv.Itoa(i)
	}

	return newTestValidatorsWithAliasesF(f, aliases)
}

func newTestValidatorsWithAliasesF(f *testing.F, aliases []string, votingPowers ...[]uint64) *testValidators {
	f.Helper()

	validators := map[string]*testValidator{}

	for i, alias := range aliases {
		votingPower := uint64(1)
		if len(votingPowers) == 1 {
			votingPower = votingPowers[0][i]
		}

		validators[alias] = newTestValidatorF(f, alias, votingPower)
	}

	return &testValidators{validators: validators}
}

func newTestValidatorF(f *testing.F, alias string, votingPower uint64) *testValidator {
	f.Helper()

	return &testValidator{
		alias:       alias,
		votingPower: votingPower,
		account:     generateTestAccountF(f),
	}
}

func generateTestAccountF(f *testing.F) *wallet.Account {
	f.Helper()

	acc, err := wallet.GenerateAccount()
	require.NoError(f, err)

	return acc
}

func newTestStateF(f *testing.F) *State {
	f.Helper()

	dir := fmt.Sprintf("/tmp/consensus-temp_%v", time.Now().UTC().Format(time.RFC3339Nano))
	err := os.Mkdir(dir, 0775)

	if err != nil {
		f.Fatal(err)
	}

	state, err := newState(path.Join(dir, "my.db"), hclog.NewNullLogger(), make(chan struct{}))
	if err != nil {
		f.Fatal(err)
	}

	f.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			f.Fatal(err)
		}
	})

	return state
}
