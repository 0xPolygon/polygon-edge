package precompiled

import (
	"errors"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_ValidatorSetPrecompile_gas(t *testing.T) {
	assert.Equal(t, uint64(150000), (&validatorSetPrecompile{}).gas(nil, nil))
}

func Test_ValidatorSetPrecompile_run_BackendNotSet(t *testing.T) {
	addr := types.StringToAddress("aaff")
	host := newDummyHost(t)
	host.context = &runtime.TxContext{
		Number: 100,
	}

	p := &validatorSetPrecompile{}
	_, err := p.run(common.PadLeftOrTrim(addr.Bytes(), 32), types.Address{}, host)

	assert.ErrorIs(t, err, errValidatorSetPrecompileNotEnabled)
}

func Test_ValidatorSetPrecompile_run_GetValidatorsForBlockError(t *testing.T) {
	desiredErr := errors.New("aaabbb")
	addr := types.StringToAddress("aaff")
	host := newDummyHost(t)
	host.context = &runtime.TxContext{
		Number: 100,
	}
	backendMock := &validatorSetBackendMock{}

	backendMock.On("GetValidatorsForBlock", uint64(100)).Return((validator.AccountSet)(nil), desiredErr)

	p := &validatorSetPrecompile{
		backend: backendMock,
	}
	_, err := p.run(common.PadLeftOrTrim(addr.Bytes(), 32), types.Address{}, host)

	assert.ErrorIs(t, err, desiredErr)
}

func Test_ValidatorSetPrecompile_run_IsValidator(t *testing.T) {
	addrGood := types.StringToAddress("a")
	addrBad := types.StringToAddress("1")
	host := newDummyHost(t)
	host.context = &runtime.TxContext{
		Number: 100,
	}
	backendMock := &validatorSetBackendMock{}

	backendMock.On("GetValidatorsForBlock", uint64(100)).Return(getDummyAccountSet(), error(nil))

	p := &validatorSetPrecompile{
		backend: backendMock,
	}

	v, err := p.run(common.PadLeftOrTrim(addrGood.Bytes(), 32), types.Address{}, host)
	require.NoError(t, err)
	assert.Equal(t, abiBoolTrue, v)

	v, err = p.run(common.PadLeftOrTrim(addrBad.Bytes(), 32), types.Address{}, host)
	require.NoError(t, err)
	assert.Equal(t, abiBoolFalse, v)
}

func Test_ValidatorSetPrecompile_run_HasQuorum(t *testing.T) {
	addrGood := [][]byte{
		types.StringToAddress("a").Bytes(),
		types.StringToAddress("b").Bytes(),
		types.StringToAddress("d").Bytes(),
	}
	addrBad1 := [][]byte{
		types.StringToAddress("a").Bytes(),
	}
	addrBad2 := [][]byte{
		types.StringToAddress("a").Bytes(),
		types.StringToAddress("0").Bytes(),
		types.StringToAddress("d").Bytes(),
	}
	host := newDummyHost(t)
	host.context = &runtime.TxContext{
		Number: 200,
	}
	backendMock := &validatorSetBackendMock{}

	backendMock.On("GetValidatorsForBlock", uint64(200)).Return(getDummyAccountSet(), error(nil))

	p := &validatorSetPrecompile{
		backend: backendMock,
	}

	bytes, err := hasQuorumAbiType.Encode(addrGood)
	require.NoError(t, err)

	v, err := p.run(bytes, types.Address{}, host)
	require.NoError(t, err)
	assert.Equal(t, abiBoolTrue, v)

	bytes, err = hasQuorumAbiType.Encode(addrBad1)
	require.NoError(t, err)

	v, err = p.run(bytes, types.Address{}, host)
	require.NoError(t, err)
	assert.Equal(t, abiBoolFalse, v)

	bytes, err = hasQuorumAbiType.Encode(addrBad2)
	require.NoError(t, err)

	v, err = p.run(bytes, types.Address{}, host)
	require.NoError(t, err)
	assert.Equal(t, abiBoolFalse, v)
}

type validatorSetBackendMock struct {
	mock.Mock
}

func (m *validatorSetBackendMock) GetValidatorsForBlock(blockNumber uint64) (validator.AccountSet, error) {
	call := m.Called(blockNumber)

	return call.Get(0).(validator.AccountSet), call.Error(1)
}

func getDummyAccountSet() validator.AccountSet {
	return validator.AccountSet{
		&validator.ValidatorMetadata{
			Address:     types.StringToAddress("a"),
			VotingPower: new(big.Int).SetUint64(1),
			IsActive:    true,
		},
		&validator.ValidatorMetadata{
			Address:     types.StringToAddress("b"),
			VotingPower: new(big.Int).SetUint64(1),
			IsActive:    true,
		},
		&validator.ValidatorMetadata{
			Address:     types.StringToAddress("c"),
			VotingPower: new(big.Int).SetUint64(1),
			IsActive:    true,
		},
		&validator.ValidatorMetadata{
			Address:     types.StringToAddress("d"),
			VotingPower: new(big.Int).SetUint64(1),
			IsActive:    true,
		},
	}
}
