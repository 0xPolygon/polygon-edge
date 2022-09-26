package fork

import (
	"encoding/json"
	"errors"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/validators"
)

const (
	// Keys in IBFT Configuration
	KeyType          = "type"
	KeyTypes         = "types"
	KeyValidatorType = "validator_type"
)

var (
	ErrUndefinedIBFTConfig = errors.New("IBFT config is not defined")
)

// IBFT Fork represents setting in params.engine.ibft of genesis.json
type IBFTFork struct {
	Type          IBFTType                 `json:"type"`
	ValidatorType validators.ValidatorType `json:"validator_type"`
	Deployment    *common.JSONNumber       `json:"deployment,omitempty"`
	From          common.JSONNumber        `json:"from"`
	To            *common.JSONNumber       `json:"to,omitempty"`

	// PoA
	Validators validators.Validators `json:"validators,omitempty"`

	// PoS
	MaxValidatorCount *common.JSONNumber `json:"maxValidatorCount,omitempty"`
	MinValidatorCount *common.JSONNumber `json:"minValidatorCount,omitempty"`
}

func (f *IBFTFork) UnmarshalJSON(data []byte) error {
	raw := struct {
		Type              IBFTType                  `json:"type"`
		ValidatorType     *validators.ValidatorType `json:"validator_type,omitempty"`
		Deployment        *common.JSONNumber        `json:"deployment,omitempty"`
		From              common.JSONNumber         `json:"from"`
		To                *common.JSONNumber        `json:"to,omitempty"`
		Validators        interface{}               `json:"validators,omitempty"`
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

	if err := json.Unmarshal(validatorsBytes, f.Validators); err != nil {
		return err
	}

	return nil
}

// GetIBFTForks returns IBFT fork configurations from chain config
func GetIBFTForks(ibftConfig map[string]interface{}) (IBFTForks, error) {
	// no fork, only specifying IBFT type in chain config
	if originalType, ok := ibftConfig[KeyType].(string); ok {
		typ, err := ParseIBFTType(originalType)
		if err != nil {
			return nil, err
		}

		validatorType := validators.ECDSAValidatorType

		if rawValType, ok := ibftConfig[KeyValidatorType].(string); ok {
			if validatorType, err = validators.ParseValidatorType(rawValType); err != nil {
				return nil, err
			}
		}

		return IBFTForks{
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
	if types, ok := ibftConfig[KeyTypes].([]interface{}); ok {
		bytes, err := json.Marshal(types)
		if err != nil {
			return nil, err
		}

		var forks IBFTForks
		if err := json.Unmarshal(bytes, &forks); err != nil {
			return nil, err
		}

		return forks, nil
	}

	return nil, ErrUndefinedIBFTConfig
}

type IBFTForks []*IBFTFork

// getByFork returns the fork in which the given height is
// it doesn't use binary search for now because number of IBFTFork is not so many
func (fs *IBFTForks) getFork(height uint64) *IBFTFork {
	for idx := len(*fs) - 1; idx >= 0; idx-- {
		fork := (*fs)[idx]

		if fork.From.Value <= height && (fork.To == nil || height <= fork.To.Value) {
			return fork
		}
	}

	return nil
}

// filterByType returns new list of IBFTFork whose type matches with the given type
func (fs *IBFTForks) filterByType(ibftType IBFTType) IBFTForks {
	filteredForks := make(IBFTForks, 0)

	for _, fork := range *fs {
		if fork.Type == ibftType {
			filteredForks = append(filteredForks, fork)
		}
	}

	return filteredForks
}
