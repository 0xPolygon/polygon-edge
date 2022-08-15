package validators

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestECDSAValidatorsMarshalJSON(t *testing.T) {
	validators := &ECDSAValidators{
		&ECDSAValidator{addr1},
		&ECDSAValidator{addr2},
	}

	res, err := json.Marshal(validators)

	assert.NoError(t, err)

	assert.Equal(
		t,
		fmt.Sprintf(
			`["%s","%s"]`,
			addr1.String(),
			addr2.String(),
		),
		string(res),
	)
}

func TestECDSAValidatorsUnmarshalJSON(t *testing.T) {
	inputStr := fmt.Sprintf(
		`["%s","%s"]`,
		addr1.String(),
		addr2.String(),
	)

	validators := new(ECDSAValidators)

	assert.NoError(
		t,
		json.Unmarshal([]byte(inputStr), validators),
	)

	assert.Equal(
		t,
		&ECDSAValidators{
			&ECDSAValidator{addr1},
			&ECDSAValidator{addr2},
		},
		validators,
	)
}

func TestBLSValidatorsMarshalJSON(t *testing.T) {
	validators := &BLSValidators{
		&BLSValidator{addr1, testBLSPubKey1},
		&BLSValidator{addr2, testBLSPubKey2},
	}

	res, err := json.Marshal(validators)

	assert.NoError(t, err)

	assert.Equal(
		t,
		fmt.Sprintf(
			`["%s","%s"]`,
			createTestBLSValidatorString(addr1, testBLSPubKey1),
			createTestBLSValidatorString(addr2, testBLSPubKey2),
		),
		string(res),
	)
}

func TestBLSValidatorsUnmarshalJSON(t *testing.T) {
	inputStr := fmt.Sprintf(
		`["%s","%s"]`,
		createTestBLSValidatorString(addr1, testBLSPubKey1),
		createTestBLSValidatorString(addr2, testBLSPubKey2),
	)

	validators := new(BLSValidators)

	assert.NoError(
		t,
		json.Unmarshal([]byte(inputStr), validators),
	)

	assert.Equal(
		t,
		&BLSValidators{
			&BLSValidator{addr1, testBLSPubKey1},
			&BLSValidator{addr2, testBLSPubKey2},
		},
		validators,
	)
}
