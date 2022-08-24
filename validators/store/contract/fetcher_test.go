package contract

import (
	"errors"
	"fmt"
	"testing"

	"github.com/0xPolygon/polygon-edge/state"
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
	var (
		ecdsaValidators = validators.NewECDSAValidatorSet(
			validators.NewECDSAValidator(addr1),
			validators.NewECDSAValidator(addr2),
		)
	)

	tests := []struct {
		name        string
		transition  *state.Transition
		from        types.Address
		expectedRes validators.Validators
		expectedErr error
	}{
		{
			name: "should return error if QueryValidators failed",
			transition: newTestTransition(
				t,
			),
			from:        types.ZeroAddress,
			expectedRes: nil,
			expectedErr: errors.New("empty input"),
		},
		{
			name: "should return ECDSA Validators",
			transition: newTestTransitionWithPredeployedStakingContract(
				t,
				ecdsaValidators,
			),
			from:        types.ZeroAddress,
			expectedRes: ecdsaValidators,
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res, err := FetchValidators(
				validators.ECDSAValidatorType,
				test.transition,
				test.from,
			)

			assert.Equal(t, test.expectedRes, res)

			if test.expectedErr != nil {
				assert.ErrorContains(t, err, test.expectedErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFetchBLSValidators(t *testing.T) {
	var (
		blsValidators = validators.NewBLSValidatorSet(
			validators.NewBLSValidator(addr1, testBLSPubKey1),
			validators.NewBLSValidator(addr2, []byte{}), // validator 2 has not set BLS Public Key
		)
	)

	tests := []struct {
		name        string
		transition  *state.Transition
		from        types.Address
		expectedRes validators.Validators
		expectedErr error
	}{
		{
			name: "should return error if QueryValidators failed",
			transition: newTestTransition(
				t,
			),
			from:        types.ZeroAddress,
			expectedRes: nil,
			expectedErr: errors.New("empty input"),
		},
		{
			name: "should return ECDSA Validators",
			transition: newTestTransitionWithPredeployedStakingContract(
				t,
				blsValidators,
			),
			from: types.ZeroAddress,
			expectedRes: validators.NewBLSValidatorSet(
				validators.NewBLSValidator(addr1, testBLSPubKey1),
			),
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res, err := FetchValidators(
				validators.BLSValidatorType,
				test.transition,
				test.from,
			)

			assert.Equal(t, test.expectedRes, res)

			if test.expectedErr != nil {
				assert.ErrorContains(t, err, test.expectedErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
