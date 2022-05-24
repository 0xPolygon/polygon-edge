package command

import (
	"errors"
	"github.com/0xPolygon/polygon-edge/helper/common"
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
	errInvalidMaxNumValidators = errors.New("maximum number of validators must be lower or equal " +
		"than MaxSafeJSInt (2^53 - 2)")
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
