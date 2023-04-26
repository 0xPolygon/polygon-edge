package polybft

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/abi"
)

const (
	// safe numbers for the test
	minStake      = 1
	minDelegation = 1
)

// getInitChildERC20PredicateInput builds input parameters for ERC20Predicate SC initialization
func getInitChildERC20PredicateInput(config *BridgeConfig) ([]byte, error) {
	//nolint:godox
	// to be fixed with EVM-541
	// TODO: @Stefan-Ethernal Temporary workaround just to be able to run cluster in non-bridge mode, until SC is fixed
	rootERC20PredicateAddr := types.StringToAddress("0xDEAD")
	rootERC20Addr := types.ZeroAddress

	if config != nil {
		rootERC20PredicateAddr = config.RootERC20PredicateAddr
		rootERC20Addr = config.RootNativeERC20Addr
	}

	params := &contractsapi.InitializeChildERC20PredicateFn{
		NewL2StateSender:          contracts.L2StateSenderContract,
		NewStateReceiver:          contracts.StateReceiverContract,
		NewRootERC20Predicate:     rootERC20PredicateAddr,
		NewChildTokenTemplate:     contracts.ChildERC20Contract,
		NewNativeTokenRootAddress: rootERC20Addr,
	}

	return params.EncodeAbi()
}

func initContract(to types.Address, input []byte, contractName string, transition *state.Transition) error {
	result := transition.Call2(contracts.SystemCaller, to, input,
		big.NewInt(0), 100_000_000)

	if result.Failed() {
		if result.Reverted() {
			unpackedRevert, err := abi.UnpackRevertError(result.ReturnValue)
			if err == nil {
				fmt.Printf("%v.initialize %v\n", contractName, unpackedRevert)
			}
		}

		return fmt.Errorf("failed to initialize %s contract. Reason: %w", contractName, result.Err)
	}

	return nil
}
