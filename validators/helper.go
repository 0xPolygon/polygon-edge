package validators

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/0xPolygon/polygon-edge/types"
)

var (
	ErrInvalidBLSValidatorFormat = errors.New("invalid validator format, expected [Validator Address]:[BLS Public Key]")
)

// NewValidatorFromType instantiates a validator by specified type
func NewValidatorFromType(t ValidatorType) (Validator, error) {
	switch t {
	case ECDSAValidatorType:
		return new(ECDSAValidator), nil
	case BLSValidatorType:
		return new(BLSValidator), nil
	}

	return nil, ErrInvalidValidatorType
}

// NewValidatorSetFromType instantiates a validators by specified type
func NewValidatorSetFromType(t ValidatorType) Validators {
	switch t {
	case ECDSAValidatorType:
		return NewECDSAValidatorSet()
	case BLSValidatorType:
		return NewBLSValidatorSet()
	}

	return nil
}

// NewECDSAValidatorSet creates Validator Set for ECDSAValidator with initialized validators
func NewECDSAValidatorSet(ecdsaValidators ...*ECDSAValidator) Validators {
	validators := make([]Validator, len(ecdsaValidators))

	for idx, val := range ecdsaValidators {
		validators[idx] = Validator(val)
	}

	return &Set{
		ValidatorType: ECDSAValidatorType,
		Validators:    validators,
	}
}

// NewBLSValidatorSet creates Validator Set for BLSValidator with initialized validators
func NewBLSValidatorSet(blsValidators ...*BLSValidator) Validators {
	validators := make([]Validator, len(blsValidators))

	for idx, val := range blsValidators {
		validators[idx] = Validator(val)
	}

	return &Set{
		ValidatorType: BLSValidatorType,
		Validators:    validators,
	}
}

// ParseValidator parses a validator represented in string
func ParseValidator(validatorType ValidatorType, validator string) (Validator, error) {
	switch validatorType {
	case ECDSAValidatorType:
		return ParseECDSAValidator(validator), nil
	case BLSValidatorType:
		return ParseBLSValidator(validator)
	default:
		// shouldn't reach here
		return nil, fmt.Errorf("invalid validator type: %s", validatorType)
	}
}

// ParseValidators parses an array of validator represented in string
func ParseValidators(validatorType ValidatorType, rawValidators []string) (Validators, error) {
	set := NewValidatorSetFromType(validatorType)
	if set == nil {
		return nil, fmt.Errorf("invalid validator type: %s", validatorType)
	}

	for _, s := range rawValidators {
		validator, err := ParseValidator(validatorType, s)
		if err != nil {
			return nil, err
		}

		if err := set.Add(validator); err != nil {
			return nil, err
		}
	}

	return set, nil
}

// ParseBLSValidator parses ECDSAValidator represented in string
func ParseECDSAValidator(validator string) *ECDSAValidator {
	return &ECDSAValidator{
		Address: types.StringToAddress(validator),
	}
}

// ParseBLSValidator parses BLSValidator represented in string
func ParseBLSValidator(validator string) (*BLSValidator, error) {
	subValues := strings.Split(validator, ":")

	if len(subValues) != 2 {
		return nil, ErrInvalidBLSValidatorFormat
	}

	addrBytes, err := hex.DecodeString(strings.TrimPrefix(subValues[0], "0x"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse address: %w", err)
	}

	pubKeyBytes, err := hex.DecodeString(strings.TrimPrefix(subValues[1], "0x"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse BLS Public Key: %w", err)
	}

	return &BLSValidator{
		Address:      types.BytesToAddress(addrBytes),
		BLSPublicKey: pubKeyBytes,
	}, nil
}
