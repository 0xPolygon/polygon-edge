package fork

import (
	"encoding/json"
	"errors"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/validators"
)

// IBFT Fork represents setting in params.engine.ibft of genesis.json
type IBFTFork struct {
	Type          IBFTType                 `json:"type"`
	ValidatorType validators.ValidatorType `json:"validator_type"`
	Deployment    *common.JSONNumber       `json:"deployment,omitempty"`
	From          common.JSONNumber        `json:"from"`
	To            *common.JSONNumber       `json:"to,omitempty"`

	// PoA
	Validators validators.Validators `json:"validators"`

	// PoS
	MaxValidatorCount *common.JSONNumber `json:"maxValidatorCount,omitempty"`
	MinValidatorCount *common.JSONNumber `json:"minValidatorCount,omitempty"`
}

func (f *IBFTFork) UnmarshalJSON(data []byte) error {
	raw := struct {
		Type              IBFTType                  `json:"type"`
		ValidatorType     *validators.ValidatorType `json:"validator_type"`
		Deployment        *common.JSONNumber        `json:"deployment,omitempty"`
		From              common.JSONNumber         `json:"from"`
		To                *common.JSONNumber        `json:"to,omitempty"`
		Validators        interface{}               `json:"validators"`
		MaxValidatorCount *common.JSONNumber        `json:"maxValidatorCount,omitempty"`
		MinValidatorCount *common.JSONNumber        `json:"minValidatorCount,omitempty"`
	}{}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	f.Type = raw.Type
	f.Deployment = raw.Deployment
	f.From = raw.From
	f.To = raw.To
	f.MaxValidatorCount = raw.MaxValidatorCount
	f.MinValidatorCount = raw.MinValidatorCount

	f.ValidatorType = validators.ECDSAValidatorType
	if raw.ValidatorType != nil {
		f.ValidatorType = *raw.ValidatorType
	}

	if raw.Validators == nil {
		return nil
	}

	f.Validators = validators.NewValidatorSetFromType(f.ValidatorType)

	validatorsBytes, err := json.Marshal(raw.Validators)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(validatorsBytes, &f.Validators); err != nil {
		return err
	}

	return nil
}

// GetIBFTForks returns IBFT fork configurations from chain config
func GetIBFTForks(ibftConfig map[string]interface{}) ([]IBFTFork, error) {
	// no fork, only specifying IBFT type in chain config
	if originalType, ok := ibftConfig["type"].(string); ok {
		typ, err := ParseIBFTType(originalType)
		if err != nil {
			return nil, err
		}

		validatorType := validators.ECDSAValidatorType
		if rawValType, ok := ibftConfig["validator_type"].(string); ok {
			if err := validatorType.FromString(rawValType); err != nil {
				return nil, err
			}
		}

		return []IBFTFork{
			{
				Type:          typ,
				Deployment:    nil,
				ValidatorType: validatorType,
				From:          common.JSONNumber{Value: 0},
				To:            nil,
			},
		}, nil
	}

	// with forks
	if types, ok := ibftConfig["types"].([]interface{}); ok {
		bytes, err := json.Marshal(types)
		if err != nil {
			return nil, err
		}

		var forks []IBFTFork
		if err := json.Unmarshal(bytes, &forks); err != nil {
			return nil, err
		}

		return forks, nil
	}

	return nil, errors.New("current IBFT type not found")
}
