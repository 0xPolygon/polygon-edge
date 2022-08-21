package contract

import (
	"fmt"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/stretchr/testify/assert"
)

func TestFetchValidators(t *testing.T) {
	// only check error handling because of the duplicated tests below
	fakeValidatorType := validators.ValidatorType("fake")
	res, err := FetchValidators(
		fakeValidatorType,
		nil,
		types.ZeroAddress,
	)

	assert.Nil(t, res)
	assert.ErrorContains(t, err, fmt.Sprintf("unsupported validator type: %s", fakeValidatorType))
}

func TestFetchECDSAValidators(t *testing.T) {
	vals := &validators.ECDSAValidators{
		validators.NewECDSAValidator(addr1),
		validators.NewECDSAValidator(addr2),
	}

	transition := newTestTransitionWithPredeployedStakingContract(
		t,
		vals,
	)

	res, err := FetchValidators(
		validators.ECDSAValidatorType,
		transition,
		types.ZeroAddress,
	)

	assert.Equal(t, vals, res)
	assert.NoError(t, err)
}

func TestFetchBLSValidators(t *testing.T) {
	vals := &validators.BLSValidators{
		validators.NewBLSValidator(addr1, testBLSPubKey1),
		validators.NewBLSValidator(addr2, []byte{}), // validator 2 has not set BLS Public Key
	}

	transition := newTestTransitionWithPredeployedStakingContract(
		t,
		vals,
	)

	res, err := FetchValidators(
		validators.BLSValidatorType,
		transition,
		types.ZeroAddress,
	)

	// only validator 1
	expected := &validators.BLSValidators{
		validators.NewBLSValidator(addr1, testBLSPubKey1),
	}

	assert.Equal(t, expected, res)
	assert.NoError(t, err)
}
