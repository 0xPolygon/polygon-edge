package abis

import (
	"github.com/umbracle/ethgo/abi"
)

var StakingABI = abi.MustNewABI(StakingJSONABI)
var StressTestABI = abi.MustNewABI(StressTestJSONABI)
