package command

import (
	"errors"
)

const (
	ConsensusFlag  = "consensus"
	NoDiscoverFlag = "no-discover"
	BootnodeFlag   = "bootnode"
	LogLevelFlag   = "log-level"
)

var (
	errInvalidValidatorRange = errors.New("minimum number of validators can not be greater than the " +
		"maximum number of validators")
	errInvalidMinNumValidators = errors.New("minimum number of validators must be greater than 0")
)

func ValidateMinMaxValidatorsNumber(minValidatorCount uint32, maxValidatorCount uint32) error {
	if minValidatorCount < 1 {
		return errInvalidMinNumValidators
	}

	if minValidatorCount > maxValidatorCount {
		return errInvalidValidatorRange
	}

	return nil
}
