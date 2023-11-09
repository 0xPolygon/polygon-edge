package command

import (
	"errors"

	"github.com/0xPolygon/polygon-edge/helper/common"
)

// Flags shared across multiple spaces
const (
	ConsensusFlag  = "consensus"
	NoDiscoverFlag = "no-discover"
	BootnodeFlag   = "bootnode"
	LogLevelFlag   = "log-level"

	ValidatorFlag         = "validators"
	ValidatorRootFlag     = "validators-path"
	ValidatorPrefixFlag   = "validators-prefix"
	MinValidatorCountFlag = "min-validator-count"
	MaxValidatorCountFlag = "max-validator-count"
)

const (
	DefaultValidatorRoot   = "./"
	DefaultValidatorPrefix = "test-chain-"
)

var (
	errInvalidValidatorRange = errors.New("minimum number of validators can not be greater than the " +
		"maximum number of validators")
	errInvalidMinNumValidators = errors.New("minimum number of validators must be greater than 0")
	errInvalidMaxNumValidators = errors.New("maximum number of validators must be lower or equal " +
		"than MaxSafeJSInt (2^53 - 2)")

	ErrValidatorNumberExceedsMax = errors.New("validator number exceeds max validator number")
)

func ValidateMinMaxValidatorsNumber(minValidatorCount uint64, maxValidatorCount uint64) error {
	if minValidatorCount < 1 {
		return errInvalidMinNumValidators
	}

	if minValidatorCount > maxValidatorCount {
		return errInvalidValidatorRange
	}

	if maxValidatorCount > common.MaxSafeJSInt {
		return errInvalidMaxNumValidators
	}

	return nil
}
