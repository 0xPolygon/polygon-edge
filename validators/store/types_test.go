package store

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/stretchr/testify/assert"
)

var (
	addr1 = types.StringToAddress("1")
	addr2 = types.StringToAddress("2")
	addr3 = types.StringToAddress("3")

	testBLSPubKey1 = validators.BLSValidatorPublicKey([]byte("bls_pubkey1"))

	ecdsaValidator1 = validators.NewECDSAValidator(addr1)
	ecdsaValidator2 = validators.NewECDSAValidator(addr2)
	blsValidator1   = validators.NewBLSValidator(addr1, testBLSPubKey1)
)

func createExampleECDSAVoteJSON(
	authorize bool,
	candidate *validators.ECDSAValidator,
	validator types.Address,
) string {
	return fmt.Sprintf(`{
		"Authorize": %t,
		"Candidate": {
			"Address": "%s"
		},
		"Validator": "%s"
	}`,
		authorize,
		candidate.Addr(),
		validator,
	)
}

func createExampleLegacyECDSAVoteJSON(
	authorize bool,
	candidate *validators.ECDSAValidator,
	validator types.Address,
) string {
	return fmt.Sprintf(`{
		"Authorize": %t,
		"Address": "%s",
		"Validator": "%s"
	}`,
		authorize,
		candidate.Addr(),
		validator,
	)
}

func createExampleBLSVoteJSON(
	authorize bool,
	candidate *validators.BLSValidator,
	validator types.Address,
) string {
	return fmt.Sprintf(`
	{
		"Authorize": %t,
		"Candidate": {
			"Address": "%s",
			"BLSPublicKey": "%s"
		},
		"Validator": "%s"
	}`,
		authorize,
		candidate.Address,
		candidate.BLSPublicKey,
		validator,
	)
}

func TestSourceTypeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sourceType SourceType
		expected   string
	}{
		{
			sourceType: Snapshot,
			expected:   "Snapshot",
		},
		{
			sourceType: Contract,
			expected:   "Contract",
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.expected, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.expected, test.sourceType.String())
		})
	}
}

func TestVoteJSONMarshal(t *testing.T) {
	t.Parallel()

	testMarshalJSON := func(
		t *testing.T,
		data interface{},
		expectedJSON string, // can be beautified
	) {
		t.Helper()

		res, err := json.Marshal(data)

		assert.NoError(t, err)
		assert.JSONEq(
			t,
			expectedJSON,
			string(res),
		)
	}

	t.Run("ECDSAValidator", func(t *testing.T) {
		t.Parallel()

		testMarshalJSON(
			t,
			&Vote{
				Authorize: true,
				Candidate: ecdsaValidator2,
				Validator: addr1,
			},
			createExampleECDSAVoteJSON(
				true,
				ecdsaValidator2,
				addr1,
			),
		)
	})

	t.Run("BLSValidator", func(t *testing.T) {
		t.Parallel()

		testMarshalJSON(
			t,
			&Vote{
				Authorize: false,
				Candidate: blsValidator1,
				Validator: addr2,
			},
			createExampleBLSVoteJSON(
				false,
				blsValidator1,
				addr2,
			),
		)
	})
}

func TestVoteJSONUnmarshal(t *testing.T) {
	t.Parallel()

	testUnmarshalJSON := func(
		t *testing.T,
		jsonStr string,
		target interface{},
		expected interface{},
	) {
		t.Helper()

		err := json.Unmarshal([]byte(jsonStr), target)

		assert.NoError(t, err)
		assert.Equal(t, expected, target)
	}

	t.Run("ECDSAValidator", func(t *testing.T) {
		t.Parallel()

		testUnmarshalJSON(
			t,
			createExampleECDSAVoteJSON(
				false,
				ecdsaValidator1,
				addr2,
			),
			&Vote{
				// need to initialize Candidate before unmarshalling
				Candidate: new(validators.ECDSAValidator),
			},
			&Vote{
				Authorize: false,
				Candidate: ecdsaValidator1,
				Validator: addr2,
			},
		)
	})

	t.Run("ECDSAValidator (legacy format)", func(t *testing.T) {
		t.Parallel()

		testUnmarshalJSON(
			t,
			createExampleLegacyECDSAVoteJSON(
				false,
				ecdsaValidator1,
				addr2,
			),
			&Vote{
				Candidate: new(validators.ECDSAValidator),
			},
			&Vote{
				Authorize: false,
				Candidate: ecdsaValidator1,
				Validator: addr2,
			},
		)
	})

	t.Run("BLSValidator", func(t *testing.T) {
		t.Parallel()

		testUnmarshalJSON(
			t,
			createExampleBLSVoteJSON(
				true,
				blsValidator1,
				addr2,
			),
			&Vote{
				// need to initialize Candidate before unmarshalling
				Candidate: new(validators.BLSValidator),
			},
			&Vote{
				Authorize: true,
				Candidate: blsValidator1,
				Validator: addr2,
			},
		)
	})
}

func TestVoteEqual(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		v1       *Vote
		v2       *Vote
		expected bool
	}{
		{
			name: "equal",
			v1: &Vote{
				Validator: addr1,
				Candidate: validators.NewECDSAValidator(addr2),
				Authorize: true,
			},
			v2: &Vote{
				Validator: addr1,
				Candidate: validators.NewECDSAValidator(addr2),
				Authorize: true,
			},
			expected: true,
		},
		{
			name: "Validators don't match with each other",
			v1: &Vote{
				Validator: addr1,
				Candidate: validators.NewECDSAValidator(addr2),
				Authorize: true,
			},
			v2: &Vote{
				Validator: addr2,
				Candidate: validators.NewECDSAValidator(addr2),
				Authorize: true,
			},
			expected: false,
		},
		{
			name: "Candidates don't match with each other",
			v1: &Vote{
				Validator: addr1,
				Candidate: validators.NewECDSAValidator(addr2),
				Authorize: true,
			},
			v2: &Vote{
				Validator: addr1,
				Candidate: validators.NewECDSAValidator(addr3),
				Authorize: true,
			},
			expected: false,
		},
		{
			name: "Authorizes don't match with each other",
			v1: &Vote{
				Validator: addr1,
				Candidate: validators.NewECDSAValidator(addr2),
				Authorize: true,
			},
			v2: &Vote{
				Validator: addr1,
				Candidate: validators.NewECDSAValidator(addr2),
				Authorize: false,
			},
			expected: false,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				test.expected,
				test.v1.Equal(test.v2),
			)
		})
	}
}

func TestVoteCopy(t *testing.T) {
	t.Parallel()

	v1 := &Vote{
		Validator: addr1,
		Candidate: validators.NewECDSAValidator(addr2),
		Authorize: true,
	}

	v2 := v1.Copy()

	assert.Equal(t, v1, v2)

	// check the addresses are different
	assert.NotSame(t, v1.Validator, v2.Validator)
	assert.NotSame(t, v1.Candidate, v2.Candidate)
	assert.NotSame(t, v1.Authorize, v2.Authorize)
}
