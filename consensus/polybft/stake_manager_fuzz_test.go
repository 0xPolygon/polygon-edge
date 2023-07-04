package polybft

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

type epochIDValidatorsF struct {
	EpochID    uint64
	Validators []*validator.ValidatorMetadata
}

type postBlockStructF struct {
	EpochID     uint64
	ValidatorID uint64
	BlockID     uint64
	StakeValue  uint64
}

type updateValidatorSetF struct {
	EpochID     uint64
	Index       uint64
	VotingPower int64
}

func FuzzTestStakeManagerPostEpoch(f *testing.F) {
	state := newTestState(f)

	seeds := []epochIDValidatorsF{
		{
			EpochID:    0,
			Validators: validator.NewTestValidators(f, 6).GetPublicIdentities(),
		},
		{
			EpochID:    1,
			Validators: validator.NewTestValidators(f, 42).GetPublicIdentities(),
		},
		{
			EpochID:    42,
			Validators: validator.NewTestValidators(f, 6).GetPublicIdentities(),
		},
	}

	for _, seed := range seeds {
		data, err := json.Marshal(seed)
		if err != nil {
			return
		}

		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		stakeManager := &stakeManager{
			logger:              hclog.NewNullLogger(),
			state:               state,
			maxValidatorSetSize: 10,
		}

		var data epochIDValidatorsF
		if err := json.Unmarshal(input, &data); err != nil {
			t.Skip(err)
		}

		invalidDataFormat := false
		for _, v := range data.Validators {
			if err := ValidateStruct(*v); err != nil {
				invalidDataFormat = true
			}
		}
		if invalidDataFormat {
			t.Skip()
		}

		err := stakeManager.PostEpoch(&PostEpochRequest{
			NewEpochID: data.EpochID,
			ValidatorSet: validator.NewValidatorSet(
				data.Validators,
				stakeManager.logger,
			),
		})
		require.NoError(t, err)
	})
}

func FuzzTestStakeManagerPostBlock(f *testing.F) {
	var (
		allAliases        = []string{"A", "B", "C", "D", "E", "F"}
		initialSetAliases = []string{"A", "B", "C", "D", "E"}
		validators        = validator.NewTestValidatorsWithAliases(f, allAliases)
		state             = newTestState(f)
	)

	seeds := []postBlockStructF{
		{
			EpochID:     0,
			ValidatorID: 1,
			BlockID:     1,
			StakeValue:  30,
		},
		{
			EpochID:     5,
			ValidatorID: 30,
			BlockID:     4,
			StakeValue:  60,
		},
		{
			EpochID:     1,
			ValidatorID: 42,
			BlockID:     11,
			StakeValue:  70,
		},
	}

	for _, seed := range seeds {
		data, err := json.Marshal(seed)
		if err != nil {
			return
		}

		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		t.Parallel()

		var data postBlockStructF
		if err := json.Unmarshal(input, &data); err != nil {
			t.Skip(err)
		}

		if err := ValidateStruct(data); err != nil {
			t.Skip(err)
		}

		if data.ValidatorID > uint64(len(initialSetAliases)-1) {
			t.Skip()
		}

		stakeManager := newStakeManager(
			hclog.NewNullLogger(),
			state,
			nil,
			wallet.NewEcdsaSigner(validators.GetValidator("A").Key()),
			types.StringToAddress("0x0001"),
			types.StringToAddress("0x0002"),
			nil,
			5,
		)

		// insert initial full validator set
		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators: newValidatorStakeMap(validators.GetPublicIdentities(initialSetAliases...)),
		}))

		receipt := &types.Receipt{
			Logs: []*types.Log{
				createTestLogForTransferEvent(
					t,
					stakeManager.validatorSetContract,
					validators.GetValidator(initialSetAliases[data.ValidatorID]).Address(),
					types.ZeroAddress,
					data.StakeValue,
				),
			},
		}

		req := &PostBlockRequest{
			FullBlock: &types.FullBlock{Block: &types.Block{Header: &types.Header{Number: data.BlockID}},
				Receipts: []*types.Receipt{receipt},
			},
			Epoch: data.EpochID,
		}
		err := stakeManager.PostBlock(req)
		require.NoError(t, err)
	})
}

func FuzzTestStakeManagerUpdateValidatorSet(f *testing.F) {
	var (
		aliases = []string{"A", "B", "C", "D", "E"}
		stakes  = []uint64{10, 10, 10, 10, 10}
	)

	validators := validator.NewTestValidatorsWithAliases(f, aliases, stakes)
	state := newTestState(f)

	stakeManager := newStakeManager(
		hclog.NewNullLogger(),
		state,
		nil,
		wallet.NewEcdsaSigner(validators.GetValidator("A").Key()),
		types.StringToAddress("0x0001"), types.StringToAddress("0x0002"),
		nil,
		10,
	)

	seeds := []updateValidatorSetF{
		{
			EpochID:     0,
			Index:       1,
			VotingPower: 30,
		},
		{
			EpochID:     1,
			Index:       4,
			VotingPower: 1,
		},
		{
			EpochID:     2,
			Index:       3,
			VotingPower: -2,
		},
	}

	for _, seed := range seeds {
		data, err := json.Marshal(seed)
		if err != nil {
			return
		}

		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		var data updateValidatorSetF
		if err := json.Unmarshal(input, &data); err != nil {
			t.Skip(err)
		}

		if err := ValidateStruct(data); err != nil {
			t.Skip(err)
		}

		if data.Index > uint64(len(aliases)-1) {
			t.Skip()
		}

		err := state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators: newValidatorStakeMap(validators.GetPublicIdentities())})
		require.NoError(t, err)

		_, err = stakeManager.UpdateValidatorSet(data.EpochID, validators.GetPublicIdentities(aliases[data.Index:]...))
		require.NoError(t, err)

		fullValidatorSet := validators.GetPublicIdentities().Copy()
		validatorToUpdate := fullValidatorSet[data.Index]
		validatorToUpdate.VotingPower = big.NewInt(data.VotingPower)

		_, err = stakeManager.UpdateValidatorSet(data.EpochID, validators.GetPublicIdentities())
		require.NoError(t, err)
	})
}

func ValidateStruct(s interface{}) (err error) {
	structType := reflect.TypeOf(s)
	if structType.Kind() != reflect.Struct {
		return errors.New("input param should be a struct")
	}

	structVal := reflect.ValueOf(s)
	fieldNum := structVal.NumField()

	for i := 0; i < fieldNum; i++ {
		field := structVal.Field(i)
		fieldName := structType.Field(i).Name

		isSet := field.IsValid() && !field.IsZero()

		if !isSet {
			err = fmt.Errorf("%w%s is not set; ", err, fieldName)
		}
	}

	return err
}
