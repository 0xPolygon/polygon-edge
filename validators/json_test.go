package validators

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestECDSAValidatorsMarshalJSON(t *testing.T) {
	t.Parallel()

	validators := &Set{
		ValidatorType: ECDSAValidatorType,
		Validators: []Validator{
			&ECDSAValidator{addr1},
			&ECDSAValidator{addr2},
		},
	}

	res, err := json.Marshal(validators)

	assert.NoError(t, err)

	assert.JSONEq(
		t,
		fmt.Sprintf(
			`[
				{
					"Address": "%s"
				},
				{
					"Address": "%s"
				}
			]`,
			addr1.String(),
			addr2.String(),
		),
		string(res),
	)
}

func TestECDSAValidatorsUnmarshalJSON(t *testing.T) {
	t.Parallel()

	inputStr := fmt.Sprintf(
		`[
			{
				"Address": "%s"
			},
			{
				"Address": "%s"
			}
		]`,
		addr1.String(),
		addr2.String(),
	)

	validators := NewECDSAValidatorSet()

	assert.NoError(
		t,
		json.Unmarshal([]byte(inputStr), validators),
	)

	assert.Equal(
		t,
		&Set{
			ValidatorType: ECDSAValidatorType,
			Validators: []Validator{
				&ECDSAValidator{addr1},
				&ECDSAValidator{addr2},
			},
		},
		validators,
	)
}

func TestBLSValidatorsMarshalJSON(t *testing.T) {
	t.Parallel()

	validators := &Set{
		ValidatorType: BLSValidatorType,
		Validators: []Validator{
			&BLSValidator{addr1, testBLSPubKey1},
			&BLSValidator{addr2, testBLSPubKey2},
		},
	}

	res, err := json.Marshal(validators)

	assert.NoError(t, err)

	assert.JSONEq(
		t,
		fmt.Sprintf(
			`[
				{
					"Address": "%s",
					"BLSPublicKey": "%s"
				},
				{
					"Address": "%s",
					"BLSPublicKey": "%s"
				}
			]`,
			addr1,
			testBLSPubKey1,
			addr2,
			testBLSPubKey2,
		),
		string(res),
	)
}

func TestBLSValidatorsUnmarshalJSON(t *testing.T) {
	t.Parallel()

	inputStr := fmt.Sprintf(
		`[
			{
				"Address": "%s",
				"BLSPublicKey": "%s"
			},
			{
				"Address": "%s",
				"BLSPublicKey": "%s"
			}
		]`,
		addr1,
		testBLSPubKey1,
		addr2,
		testBLSPubKey2,
	)

	validators := NewBLSValidatorSet()

	assert.NoError(
		t,
		json.Unmarshal([]byte(inputStr), validators),
	)

	assert.Equal(
		t,
		&Set{
			ValidatorType: BLSValidatorType,
			Validators: []Validator{
				&BLSValidator{addr1, testBLSPubKey1},
				&BLSValidator{addr2, testBLSPubKey2},
			},
		},
		validators,
	)
}
