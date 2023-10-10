package signer

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/stretchr/testify/assert"
)

func JSONMarshalHelper(t *testing.T, extra *IstanbulExtra) string {
	t.Helper()

	res, err := json.Marshal(extra)

	assert.NoError(t, err)

	return string(res)
}

func TestIstanbulExtraMarshalAndUnmarshal(t *testing.T) {
	tests := []struct {
		name  string
		extra *IstanbulExtra
	}{
		{
			name: "ECDSAExtra",
			extra: &IstanbulExtra{
				Validators: validators.NewECDSAValidatorSet(
					validators.NewECDSAValidator(
						testAddr1,
					),
				),
				ProposerSeal: testProposerSeal,
				CommittedSeals: &SerializedSeal{
					[]byte{0x1},
					[]byte{0x2},
				},
				ParentCommittedSeals: &SerializedSeal{
					[]byte{0x3},
					[]byte{0x4},
				},
			},
		},
		{
			name: "ECDSAExtra without ParentCommittedSeals",
			extra: &IstanbulExtra{
				Validators: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
				),
				ProposerSeal: testProposerSeal,
				CommittedSeals: &SerializedSeal{
					[]byte{0x1},
					[]byte{0x2},
				},
			},
		},
		{
			name: "BLSExtra",
			extra: &IstanbulExtra{
				Validators: validators.NewBLSValidatorSet(
					blsValidator1,
				),
				ProposerSeal: testProposerSeal,
				CommittedSeals: &AggregatedSeal{
					Bitmap:    new(big.Int).SetBytes([]byte{0x8}),
					Signature: []byte{0x1},
				},
				ParentCommittedSeals: &AggregatedSeal{
					Bitmap:    new(big.Int).SetBytes([]byte{0x9}),
					Signature: []byte{0x2},
				},
			},
		},
		{
			name: "BLSExtra without ParentCommittedSeals",
			extra: &IstanbulExtra{
				Validators: validators.NewBLSValidatorSet(
					blsValidator1,
				),
				ProposerSeal: testProposerSeal,
				CommittedSeals: &AggregatedSeal{
					Bitmap:    new(big.Int).SetBytes([]byte{0x8}),
					Signature: []byte{0x1},
				},
				ParentCommittedSeals: &AggregatedSeal{
					Bitmap:    new(big.Int).SetBytes([]byte{0x9}),
					Signature: []byte{0x2},
				},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			// create original data
			originalExtraJSON := JSONMarshalHelper(t, test.extra)

			bytesData := test.extra.MarshalRLPTo(nil)
			err := test.extra.UnmarshalRLP(bytesData)
			assert.NoError(t, err)

			// make sure all data is recovered
			assert.Equal(
				t,
				originalExtraJSON,
				JSONMarshalHelper(t, test.extra),
			)
		})
	}
}

func Test_packProposerSealIntoExtra(t *testing.T) {
	newProposerSeal := []byte("new proposer seal")

	tests := []struct {
		name  string
		extra *IstanbulExtra
	}{
		{
			name: "ECDSAExtra",
			extra: &IstanbulExtra{
				Validators: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
				),
				ProposerSeal: testProposerSeal,
				CommittedSeals: &SerializedSeal{
					[]byte{0x1},
					[]byte{0x2},
				},
				ParentCommittedSeals: &SerializedSeal{
					[]byte{0x3},
					[]byte{0x4},
				},
			},
		},
		{
			name: "ECDSAExtra without ParentCommittedSeals",
			extra: &IstanbulExtra{
				Validators: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
				),
				ProposerSeal: testProposerSeal,
				CommittedSeals: &SerializedSeal{
					[]byte{0x1},
					[]byte{0x2},
				},
			},
		},
		{
			name: "BLSExtra",
			extra: &IstanbulExtra{
				Validators: validators.NewBLSValidatorSet(
					blsValidator1,
				),
				ProposerSeal: testProposerSeal,
				CommittedSeals: &AggregatedSeal{
					Bitmap:    new(big.Int).SetBytes([]byte{0x8}),
					Signature: []byte{0x1},
				},
				ParentCommittedSeals: &AggregatedSeal{
					Bitmap:    new(big.Int).SetBytes([]byte{0x9}),
					Signature: []byte{0x2},
				},
			},
		},
		{
			name: "BLSExtra without ParentCommittedSeals",
			extra: &IstanbulExtra{
				Validators: validators.NewBLSValidatorSet(
					blsValidator1,
				),
				ProposerSeal: testProposerSeal,
				CommittedSeals: &AggregatedSeal{
					Bitmap:    new(big.Int).SetBytes([]byte{0x8}),
					Signature: []byte{0x1},
				},
				ParentCommittedSeals: &AggregatedSeal{
					Bitmap:    new(big.Int).SetBytes([]byte{0x9}),
					Signature: []byte{0x2},
				},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			originalProposerSeal := test.extra.ProposerSeal

			// create expected data
			test.extra.ProposerSeal = newProposerSeal
			expectedJSON := JSONMarshalHelper(t, test.extra)
			test.extra.ProposerSeal = originalProposerSeal

			newExtraBytes := packProposerSealIntoExtra(
				// prepend IstanbulExtraHeader to parse
				append(
					make([]byte, IstanbulExtraVanity),
					test.extra.MarshalRLPTo(nil)...,
				),
				newProposerSeal,
			)

			assert.NoError(
				t,
				test.extra.UnmarshalRLP(newExtraBytes[IstanbulExtraVanity:]),
			)

			// check json of decoded data matches with the original data
			jsonData := JSONMarshalHelper(t, test.extra)

			assert.Equal(
				t,
				expectedJSON,
				jsonData,
			)
		})
	}
}

func Test_packCommittedSealsAndRoundNumberIntoExtra(t *testing.T) {
	tests := []struct {
		name              string
		extra             *IstanbulExtra
		newCommittedSeals Seals
		roundNumber       *uint64
	}{
		{
			name: "ECDSAExtra",
			extra: &IstanbulExtra{
				Validators: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
				),
				ProposerSeal: testProposerSeal,
				CommittedSeals: &SerializedSeal{
					[]byte{0x1},
					[]byte{0x2},
				},
				ParentCommittedSeals: &SerializedSeal{
					[]byte{0x3},
					[]byte{0x4},
				},
			},
			newCommittedSeals: &SerializedSeal{
				[]byte{0x3},
				[]byte{0x4},
			},
			roundNumber: nil,
		},
		{
			name: "ECDSAExtra without ParentCommittedSeals",
			extra: &IstanbulExtra{
				Validators: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
				),
				ProposerSeal: testProposerSeal,
				CommittedSeals: &SerializedSeal{
					[]byte{0x1},
					[]byte{0x2},
				},
			},
			newCommittedSeals: &SerializedSeal{
				[]byte{0x3},
				[]byte{0x4},
			},
			roundNumber: nil,
		},
		{
			name: "BLSExtra",
			extra: &IstanbulExtra{
				Validators: validators.NewBLSValidatorSet(
					blsValidator1,
				),
				ProposerSeal: testProposerSeal,
				CommittedSeals: &AggregatedSeal{
					Bitmap:    new(big.Int).SetBytes([]byte{0x8}),
					Signature: []byte{0x1},
				},
				ParentCommittedSeals: &AggregatedSeal{
					Bitmap:    new(big.Int).SetBytes([]byte{0x9}),
					Signature: []byte{0x2},
				},
			},
			newCommittedSeals: &AggregatedSeal{
				Bitmap:    new(big.Int).SetBytes([]byte{0xa}),
				Signature: []byte{0x2},
			},
			roundNumber: nil,
		},
		{
			name: "BLSExtra without ParentCommittedSeals",
			extra: &IstanbulExtra{
				Validators: validators.NewBLSValidatorSet(
					blsValidator1,
				),
				ProposerSeal: testProposerSeal,
				CommittedSeals: &AggregatedSeal{
					Bitmap:    new(big.Int).SetBytes([]byte{0x8}),
					Signature: []byte{0x1},
				},
				ParentCommittedSeals: &AggregatedSeal{
					Bitmap:    new(big.Int).SetBytes([]byte{0x9}),
					Signature: []byte{0x2},
				},
			},
			newCommittedSeals: &AggregatedSeal{
				Bitmap:    new(big.Int).SetBytes([]byte{0xa}),
				Signature: []byte{0x2},
			},
			roundNumber: nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			originalCommittedSeals := test.extra.CommittedSeals

			// create expected data
			test.extra.CommittedSeals = test.newCommittedSeals
			expectedJSON := JSONMarshalHelper(t, test.extra)
			test.extra.CommittedSeals = originalCommittedSeals

			// update committed seals
			newExtraBytes := packCommittedSealsAndRoundNumberIntoExtra(
				// prepend IstanbulExtraHeader
				append(
					make([]byte, IstanbulExtraVanity),
					test.extra.MarshalRLPTo(nil)...,
				),
				test.newCommittedSeals,
				test.roundNumber,
			)

			// decode RLP data
			assert.NoError(
				t,
				test.extra.UnmarshalRLP(newExtraBytes[IstanbulExtraVanity:]),
			)

			// check json of decoded data matches with the original data
			jsonData := JSONMarshalHelper(t, test.extra)

			assert.Equal(
				t,
				expectedJSON,
				jsonData,
			)
		})
	}
}

func Test_unmarshalRLPForParentCS(t *testing.T) {
	tests := []struct {
		name        string
		extra       *IstanbulExtra
		targetExtra *IstanbulExtra
	}{
		{
			name: "ECDSAExtra",
			extra: &IstanbulExtra{
				Validators: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
				),
				ProposerSeal: testProposerSeal,
				CommittedSeals: &SerializedSeal{
					[]byte{0x1},
					[]byte{0x2},
				},
				ParentCommittedSeals: &SerializedSeal{
					[]byte{0x3},
					[]byte{0x4},
				},
			},
			targetExtra: &IstanbulExtra{
				ParentCommittedSeals: &SerializedSeal{},
			},
		},
		{
			name: "BLSExtra",
			extra: &IstanbulExtra{
				Validators: validators.NewBLSValidatorSet(
					blsValidator1,
				),
				ProposerSeal: testProposerSeal,
				CommittedSeals: &AggregatedSeal{
					Bitmap:    new(big.Int).SetBytes([]byte{0x8}),
					Signature: []byte{0x1},
				},
				ParentCommittedSeals: &AggregatedSeal{
					Bitmap:    new(big.Int).SetBytes([]byte{0x9}),
					Signature: []byte{0x2},
				},
			},
			targetExtra: &IstanbulExtra{
				ParentCommittedSeals: &AggregatedSeal{},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			bytesData := test.extra.MarshalRLPTo(nil)

			assert.NoError(t, test.targetExtra.unmarshalRLPForParentCS(bytesData))

			// make sure all data is recovered
			assert.Equal(
				t,
				test.extra.ParentCommittedSeals,
				test.targetExtra.ParentCommittedSeals,
			)
		})
	}
}

func Test_putIbftExtra(t *testing.T) {
	tests := []struct {
		name   string
		header *types.Header
		extra  *IstanbulExtra
	}{
		{
			name: "ECDSAExtra",
			header: &types.Header{
				ExtraData: []byte{},
			},
			extra: &IstanbulExtra{
				Validators: validators.NewECDSAValidatorSet(
					ecdsaValidator1,
				),
				ProposerSeal: testProposerSeal,
				CommittedSeals: &SerializedSeal{
					[]byte{0x1},
					[]byte{0x2},
				},
				ParentCommittedSeals: &SerializedSeal{
					[]byte{0x3},
					[]byte{0x4},
				},
			},
		},
		{
			name: "BLSExtra",
			header: &types.Header{
				ExtraData: make([]byte, IstanbulExtraVanity+10),
			},
			extra: &IstanbulExtra{
				Validators: validators.NewBLSValidatorSet(
					blsValidator1,
				),
				ProposerSeal: testProposerSeal,
				CommittedSeals: &AggregatedSeal{
					Bitmap:    new(big.Int).SetBytes([]byte{0x8}),
					Signature: []byte{0x1},
				},
				ParentCommittedSeals: &AggregatedSeal{
					Bitmap:    new(big.Int).SetBytes([]byte{0x9}),
					Signature: []byte{0x2},
				},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			putIbftExtra(test.header, test.extra)

			expectedExtraHeader := make([]byte, IstanbulExtraVanity)
			expectedExtraBody := test.extra.MarshalRLPTo(nil)
			expectedExtra := append(expectedExtraHeader, expectedExtraBody...) //nolint:makezero

			assert.Equal(
				t,
				expectedExtra,
				test.header.ExtraData,
			)
		})
	}
}
