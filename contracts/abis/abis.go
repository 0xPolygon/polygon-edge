package abis

import (
	"github.com/umbracle/go-web3/abi"
)

// Predeployed system contract ABI
var (
	// ValidatorSet contract abi
	ValidatorSetABI = abi.MustNewABI(ValidatorSetJSONABI)
	// bridge contract abi
	BridgeABI = abi.MustNewABI(BridgeJSONABI)
	// vault contract abi
	VaultABI = abi.MustNewABI(VaultJSONABI)
)

// Temporarily deployed contract ABI
var StressTestABI = abi.MustNewABI(StressTestJSONABI)
