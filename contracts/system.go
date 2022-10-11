package contracts

import "github.com/ethereum/go-ethereum/common"

var (
	// ValidatorSetContract is address of validator set contract deployed to child chain
	ValidatorSetContract = common.Address{0x1}
	// StateReceiverContract is address of bridge contract deployed to child chain
	StateReceiverContract = common.HexToAddress("0x0000000000000000000000000000000000001001")
	// NativeTokenContract is an address of bridge contract used for transferring native tokens
	NativeTokenContract = common.HexToAddress("0x0000000000000000000000000000000000001010")
	// NativeTransferPrecompile is an address of native transfer precompile
	NativeTransferPrecompile = common.HexToAddress("0x0000000000000000000000000000000000002020")
)
