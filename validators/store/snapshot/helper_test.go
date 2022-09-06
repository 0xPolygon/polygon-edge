package snapshot

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/stretchr/testify/assert"
)

var (
	addr1          = types.StringToAddress("1")
	addr2          = types.StringToAddress("2")
	addr3          = types.StringToAddress("3")
	testBLSPubKey1 = validators.BLSValidatorPublicKey([]byte("bls_pubkey1"))
	testBLSPubKey2 = validators.BLSValidatorPublicKey([]byte("bls_pubkey2"))
	testBLSPubKey3 = validators.BLSValidatorPublicKey([]byte("bls_pubkey3"))

	ecdsaValidator1 = validators.NewECDSAValidator(addr1)
	ecdsaValidator2 = validators.NewECDSAValidator(addr2)
	ecdsaValidator3 = validators.NewECDSAValidator(addr3)
	blsValidator1   = validators.NewBLSValidator(addr1, testBLSPubKey1)
	blsValidator2   = validators.NewBLSValidator(addr2, testBLSPubKey2)
	blsValidator3   = validators.NewBLSValidator(addr3, testBLSPubKey3)
)

func Test_isAuthorize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		nonce        types.Nonce
		expectedFlag bool
		expectedErr  error
	}{
		{
			name:         "nonceAuthVote",
			nonce:        nonceAuthVote,
			expectedFlag: true,
			expectedErr:  nil,
		},
		{
			name:         "nonceDropVote",
			nonce:        nonceDropVote,
			expectedFlag: false,
			expectedErr:  nil,
		},
		{
			name:         "invalid nonce",
			nonce:        types.Nonce{0x1},
			expectedFlag: false,
			expectedErr:  ErrIncorrectNonce,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			res, err := isAuthorize(test.nonce)

			assert.Equal(t, test.expectedFlag, res)
			assert.ErrorIs(t, test.expectedErr, err)
		})
	}
}

func Test_shouldProcessVote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		validators validators.Validators
		candidate  types.Address
		voteAction bool
		expected   bool
	}{
		// ECDSA
		{
			name: "ECDSA: vote for addition when the candidate isn't in validators",
			validators: validators.NewECDSAValidatorSet(
				ecdsaValidator1,
			),
			candidate:  ecdsaValidator2.Addr(),
			voteAction: true,
			expected:   true,
		},
		{
			name: "ECDSA: vote for addition when the candidate is already in validators",
			validators: validators.NewECDSAValidatorSet(
				ecdsaValidator1,
				ecdsaValidator2,
			),
			candidate:  ecdsaValidator2.Addr(),
			voteAction: true,
			expected:   false,
		},
		{
			name: "ECDSA: vote for deletion when the candidate is in validators",
			validators: validators.NewECDSAValidatorSet(
				ecdsaValidator1,
				ecdsaValidator2,
			),
			candidate:  ecdsaValidator2.Addr(),
			voteAction: false,
			expected:   true,
		},
		{
			name: "ECDSA: vote for deletion when the candidate isn't in validators",
			validators: validators.NewECDSAValidatorSet(
				ecdsaValidator1,
			),
			candidate:  ecdsaValidator2.Addr(),
			voteAction: false,
			expected:   false,
		},
		// BLS
		{
			name: "BLS: vote for addition when the candidate isn't in validators",
			validators: validators.NewBLSValidatorSet(
				blsValidator1,
			),
			candidate:  blsValidator2.Addr(),
			voteAction: true,
			expected:   true,
		},
		{
			name: "BLS: vote for addition when the candidate is already in validators",
			validators: validators.NewBLSValidatorSet(
				blsValidator1,
				blsValidator2,
			),
			candidate:  blsValidator2.Addr(),
			voteAction: true,
			expected:   false,
		},
		{
			name: "BLS: vote for deletion when the candidate is in validators",
			validators: validators.NewBLSValidatorSet(
				blsValidator1,
				blsValidator2,
			),
			candidate:  blsValidator1.Addr(),
			voteAction: false,
			expected:   true,
		},
		{
			name: "BLS: vote for deletion when the candidate isn't in validators",
			validators: validators.NewBLSValidatorSet(
				blsValidator1,
			),
			candidate:  blsValidator2.Addr(),
			voteAction: false,
			expected:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				test.expected,
				shouldProcessVote(test.validators, test.candidate, test.voteAction),
			)
		})
	}
}

func Test_addsOrDelsCandidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// inputs
		validators   validators.Validators
		candidate    validators.Validator
		updateAction bool
		// outputs
		expectedErr   error
		newValidators validators.Validators
	}{
		// ECDSA
		{
			name: "ECDSA: addition when the candidate isn't in validators",
			validators: validators.NewECDSAValidatorSet(
				ecdsaValidator1,
			),
			candidate:    ecdsaValidator2,
			updateAction: true,
			expectedErr:  nil,
			newValidators: validators.NewECDSAValidatorSet(
				ecdsaValidator1,
				ecdsaValidator2,
			),
		},
		{
			name: "ECDSA: addition when the candidate is already in validators",
			validators: validators.NewECDSAValidatorSet(
				ecdsaValidator1,
				ecdsaValidator2,
			),
			candidate:    ecdsaValidator2,
			updateAction: true,
			expectedErr:  validators.ErrValidatorAlreadyExists,
			newValidators: validators.NewECDSAValidatorSet(
				ecdsaValidator1,
				ecdsaValidator2,
			),
		},
		{
			name: "ECDSA: deletion when the candidate is in validators",
			validators: validators.NewECDSAValidatorSet(
				ecdsaValidator1,
				ecdsaValidator2,
			),
			candidate:    ecdsaValidator1,
			updateAction: false,
			expectedErr:  nil,
			newValidators: validators.NewECDSAValidatorSet(
				ecdsaValidator2,
			),
		},
		{
			name: "ECDSA: deletion when the candidate isn't in validators",
			validators: validators.NewECDSAValidatorSet(
				ecdsaValidator2,
			),
			candidate:    ecdsaValidator1,
			updateAction: false,
			expectedErr:  validators.ErrValidatorNotFound,
			newValidators: validators.NewECDSAValidatorSet(
				ecdsaValidator2,
			),
		},
		// BLS
		{
			name: "BLS: addition when the candidate isn't in validators",
			validators: validators.NewBLSValidatorSet(
				blsValidator1,
			),
			candidate:    blsValidator2,
			updateAction: true,
			expectedErr:  nil,
			newValidators: validators.NewBLSValidatorSet(
				blsValidator1,
				blsValidator2,
			),
		},
		{
			name: "BLS: addition when the candidate is already in validators",
			validators: validators.NewBLSValidatorSet(
				blsValidator1,
				blsValidator2,
			),
			candidate:    blsValidator2,
			updateAction: true,
			expectedErr:  validators.ErrValidatorAlreadyExists,
			newValidators: validators.NewBLSValidatorSet(
				blsValidator1,
				blsValidator2,
			),
		},
		{
			name: "BLS: deletion when the candidate is in validators",
			validators: validators.NewBLSValidatorSet(
				blsValidator1,
				blsValidator2,
			),
			candidate:    blsValidator1,
			updateAction: false,
			expectedErr:  nil,
			newValidators: validators.NewBLSValidatorSet(
				blsValidator2,
			),
		},
		{
			name: "BLS: deletion when the candidate is in validators",
			validators: validators.NewBLSValidatorSet(
				blsValidator2,
			),
			candidate:    blsValidator1,
			updateAction: false,
			expectedErr:  validators.ErrValidatorNotFound,
			newValidators: validators.NewBLSValidatorSet(
				blsValidator2,
			),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := addsOrDelsCandidate(
				test.validators,
				test.candidate,
				test.updateAction,
			)

			assert.ErrorIs(t, test.expectedErr, err)
			assert.Equal(t, test.newValidators, test.validators)
		})
	}
}
