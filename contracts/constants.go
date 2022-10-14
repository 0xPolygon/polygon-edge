package contracts

import "github.com/0xPolygon/polygon-edge/types"

var (
	// ValidatorSetContract is address of validator set contract deployed to child chain
	ValidatorSetContract = types.Address{0x1}
	// StateReceiverContract is address of bridge contract deployed to child chain
	StateReceiverContract = types.StringToAddress("0x0000000000000000000000000000000000001001")
	// NativeTokenContract is an address of bridge contract used for transferring native tokens
	NativeTokenContract = types.StringToAddress("0x0000000000000000000000000000000000001010")
	// NativeTransferPrecompile is an address of native transfer precompile
	NativeTransferPrecompile = types.StringToAddress("0x0000000000000000000000000000000000002020")
	// BLSAggSigsVerificationPrecompile is an address of BLS aggregated signatures verificatin precompile
	BLSAggSigsVerificationPrecompile = types.StringToAddress("0x0000000000000000000000000000000000002030")
)
