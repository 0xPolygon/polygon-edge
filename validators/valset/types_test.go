package valset

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/stretchr/testify/assert"
)

var (
	addr1 = types.StringToAddress("1")
	addr2 = types.StringToAddress("2")
	addr3 = types.StringToAddress("3")
)

func TestSourceTypeString(t *testing.T) {
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
		t.Run(test.expected, func(t *testing.T) {
			assert.Equal(t, test.expected, test.sourceType.String())
		})
	}
}

func TestVoteEqual(t *testing.T) {
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
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(
				t,
				test.expected,
				test.v1.Equal(test.v2),
			)
		})
	}
}

func TestVoteCopy(t *testing.T) {
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
