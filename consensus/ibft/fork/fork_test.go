package fork

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/0xPolygon/polygon-edge/helper/common"
	testHelper "github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/stretchr/testify/assert"
)

func TestIBFTForkUnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     string
		expected *IBFTFork
		err      error
	}{
		{
			name: "should parse without validator type and validators",
			data: fmt.Sprintf(`{
				"type": "%s",
				"from": %d
			}`, PoA, 0),
			expected: &IBFTFork{
				Type:              PoA,
				ValidatorType:     validators.ECDSAValidatorType,
				From:              common.JSONNumber{Value: 0},
				To:                nil,
				Validators:        nil,
				MaxValidatorCount: nil,
				MinValidatorCount: nil,
			},
		},
		{
			name: "should parse without validators",
			data: fmt.Sprintf(`{
				"type": "%s",
				"from": %d,
				"to": %d,
				"maxValidatorCount": %d,
				"MinValidatorCount": %d
			}`, PoS, 10, 15, 100, 1),
			expected: &IBFTFork{
				Type:              PoS,
				ValidatorType:     validators.ECDSAValidatorType,
				From:              common.JSONNumber{Value: 10},
				To:                &common.JSONNumber{Value: 15},
				Validators:        nil,
				MaxValidatorCount: &common.JSONNumber{Value: 100},
				MinValidatorCount: &common.JSONNumber{Value: 1},
			},
		},
		{
			name: "should parse without validators",
			data: fmt.Sprintf(`{
				"type": "%s",
				"validator_type": "%s",
				"validators": [
					{
						"Address": "%s"
					},
					{
						"Address": "%s"
					}
				],
				"from": %d,
				"to": %d
			}`,
				PoA, validators.ECDSAValidatorType,
				types.StringToAddress("1"), types.StringToAddress("2"),
				16, 20,
			),
			expected: &IBFTFork{
				Type:          PoA,
				ValidatorType: validators.ECDSAValidatorType,
				From:          common.JSONNumber{Value: 16},
				To:            &common.JSONNumber{Value: 20},
				Validators: validators.NewECDSAValidatorSet(
					validators.NewECDSAValidator(types.StringToAddress("1")),
					validators.NewECDSAValidator(types.StringToAddress("2")),
				),
				MaxValidatorCount: nil,
				MinValidatorCount: nil,
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fork := &IBFTFork{}

			err := json.Unmarshal([]byte(test.data), fork)

			testHelper.AssertErrorMessageContains(t, test.err, err)

			assert.Equal(t, test.expected, fork)
		})
	}
}

func TestGetIBFTForks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config map[string]interface{}
		res    IBFTForks
		err    error
	}{
		{
			name: "should return error if invalid type is set in type",
			config: map[string]interface{}{
				"type": "invalid",
			},
			res: nil,
			err: errors.New("invalid IBFT type invalid"),
		},
		{
			name: "should return a single fork for ECDSA if IBFTConfig has type but it doesn't have validator type",
			config: map[string]interface{}{
				"type": "PoA",
			},
			res: IBFTForks{
				{
					Type:          PoA,
					ValidatorType: validators.ECDSAValidatorType,
					Deployment:    nil,
					From:          common.JSONNumber{Value: 0},
					To:            nil,
				},
			},
			err: nil,
		},
		{
			name: "should return a single fork for ECDSA if IBFTConfig has type and validator type",
			config: map[string]interface{}{
				"type":           "PoS",
				"validator_type": "bls",
			},
			res: IBFTForks{
				{
					Type:          PoS,
					ValidatorType: validators.BLSValidatorType,
					Deployment:    nil,
					From:          common.JSONNumber{Value: 0},
					To:            nil,
				},
			},
			err: nil,
		},
		{
			name: "should return multiple forks",
			config: map[string]interface{}{
				"types": []interface{}{
					map[string]interface{}{
						"type":           "PoA",
						"validator_type": "ecdsa",
						"from":           0,
						"to":             10,
					},
					map[string]interface{}{
						"type":           "PoA",
						"validator_type": "bls",
						"from":           11,
					},
				},
			},
			res: IBFTForks{
				{
					Type:          PoA,
					ValidatorType: validators.ECDSAValidatorType,
					Deployment:    nil,
					From:          common.JSONNumber{Value: 0},
					To:            &common.JSONNumber{Value: 10},
				},
				{
					Type:          PoA,
					ValidatorType: validators.BLSValidatorType,
					Deployment:    nil,
					From:          common.JSONNumber{Value: 11},
					To:            nil,
				},
			},
			err: nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			res, err := GetIBFTForks(test.config)

			assert.Equal(
				t,
				test.res,
				res,
			)

			testHelper.AssertErrorMessageContains(
				t,
				test.err,
				err,
			)
		})
	}
}

func TestIBFTForks_getFork(t *testing.T) {
	t.Parallel()

	forks := IBFTForks{
		{
			From: common.JSONNumber{Value: 0},
			To:   &common.JSONNumber{Value: 10},
		},
		{
			From: common.JSONNumber{Value: 11},
			To:   &common.JSONNumber{Value: 50},
		},
		{
			From: common.JSONNumber{Value: 51},
		},
	}

	tests := []struct {
		name     string
		height   uint64
		expected *IBFTFork
	}{
		{
			name:     "should return the first fork if height is 0",
			height:   0,
			expected: forks[0],
		},
		{
			name:     "should return the first fork if height is 1",
			height:   1,
			expected: forks[0],
		},
		{
			name:     "should return the first fork if height is 10",
			height:   10,
			expected: forks[0],
		},
		{
			name:     "should return the first fork if height is 11",
			height:   11,
			expected: forks[1],
		},
		{
			name:     "should return the first fork if height is 50",
			height:   50,
			expected: forks[1],
		},
		{
			name:     "should return the first fork if height is 51",
			height:   51,
			expected: forks[2],
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				test.expected,
				forks.getFork(test.height),
			)
		})
	}
}

func TestIBFTForks_filterByType(t *testing.T) {
	t.Parallel()

	forks := IBFTForks{
		{
			Type: PoA,
			From: common.JSONNumber{Value: 0},
			To:   &common.JSONNumber{Value: 10},
		},
		{
			Type: PoS,
			From: common.JSONNumber{Value: 11},
			To:   &common.JSONNumber{Value: 20},
		},
		{
			Type: PoA,
			From: common.JSONNumber{Value: 21},
			To:   &common.JSONNumber{Value: 30},
		},
		{
			Type: PoS,
			From: common.JSONNumber{Value: 31},
		},
	}

	assert.Equal(
		t,
		IBFTForks{
			forks[0],
			forks[2],
		},
		forks.filterByType(PoA),
	)

	assert.Equal(
		t,
		IBFTForks{
			forks[1],
			forks[3],
		},
		forks.filterByType(PoS),
	)
}
