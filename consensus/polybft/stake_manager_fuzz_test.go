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
	"github.com/stretchr/testify/mock"
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
		{
			EpochID:     7,
			ValidatorID: 1,
			BlockID:     2,
			StakeValue:  10,
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

		validatorSetAddr := types.StringToAddress("0x0001")

		bcMock := new(blockchainMock)
		for i := 0; i < int(data.BlockID); i++ {
			bcMock.On("CurrentHeader").Return(&types.Header{Number: 0})
			bcMock.On("GetHeaderByNumber", mock.Anything).Return(&types.Header{Hash: types.Hash{6, 4}}, true).Once()
			bcMock.On("GetReceiptsByHash", mock.Anything).Return([]*types.Receipt{{}}, error(nil)).Once()
		}

		// insert initial full validator set
		require.NoError(t, state.StakeStore.insertFullValidatorSet(validatorSetState{
			Validators: newValidatorStakeMap(validators.GetPublicIdentities(initialSetAliases...)),
		}, nil))

		stakeManager, err := newStakeManager(
			hclog.NewNullLogger(),
			state,
			nil,
			wallet.NewEcdsaSigner(validators.GetValidator("A").Key()),
			validatorSetAddr,
			types.StringToAddress("0x0002"),
			bcMock,
			nil,
			5,
			nil,
		)
		require.NoError(t, err)

		header := &types.Header{Number: data.BlockID}
		require.NoError(t, stakeManager.ProcessLog(header, convertLog(createTestLogForTransferEvent(
			t,
			validatorSetAddr,
			validators.GetValidator(initialSetAliases[data.ValidatorID]).Address(),
			types.ZeroAddress,
			data.StakeValue,
		)), nil))

		require.NoError(t, stakeManager.PostBlock(&PostBlockRequest{
			FullBlock: &types.FullBlock{Block: &types.Block{Header: &types.Header{Number: data.BlockID}}},
			Epoch:     data.EpochID,
		}))
	})
}

func FuzzTestStakeManagerUpdateValidatorSet(f *testing.F) {
	var (
		aliases = []string{"A", "B", "C", "D", "E"}
		stakes  = []uint64{10, 10, 10, 10, 10}
	)

	validators := validator.NewTestValidatorsWithAliases(f, aliases, stakes)
	state := newTestState(f)

	bcMock := new(blockchainMock)
	bcMock.On("CurrentHeader").Return(&types.Header{Number: 0})

	err := state.StakeStore.insertFullValidatorSet(validatorSetState{
		Validators: newValidatorStakeMap(validators.GetPublicIdentities())}, nil)
	require.NoError(f, err)

	stakeManager, err := newStakeManager(
		hclog.NewNullLogger(),
		state,
		nil,
		wallet.NewEcdsaSigner(validators.GetValidator("A").Key()),
		types.StringToAddress("0x0001"), types.StringToAddress("0x0002"),
		bcMock,
		nil,
		10,
		nil,
	)
	require.NoError(f, err)

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
			Validators: newValidatorStakeMap(validators.GetPublicIdentities())}, nil)
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
