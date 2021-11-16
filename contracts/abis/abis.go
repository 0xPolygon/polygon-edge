package abis

import (
	"github.com/umbracle/go-web3/abi"
)

var StakingABI = abi.MustNewABI(StakingJSONABI)
var StressTestABI = abi.MustNewABI(StressTestJSONABI)
