package contracts

import "github.com/0xPolygon/polygon-edge/types"

var (
	// ValidatorSetContract is an address of validator set contract deployed to child chain
	ValidatorSetContract = types.StringToAddress("0x101")
	// BLSContract is an address of BLS contract on the child chain
	BLSContract = types.StringToAddress("0x102")
	// MerkleContract is an address of Merkle contract on the child chain
	MerkleContract = types.StringToAddress("0x103")
	// StateReceiverContract is an address of bridge contract on the child chain
	StateReceiverContract = types.StringToAddress("0x1001")
	// NativeERC20TokenContract is an address of bridge contract (used for transferring ERC20 native tokens on child chain)
	NativeERC20TokenContract = types.StringToAddress("0x1010")
	// L2StateSenderContract is an address of bridge contract to the rootchain
	L2StateSenderContract = types.StringToAddress("0x1002")
	// ChildERC20Contract is an address of bridgable ERC20 token contract on the child chain
	ChildERC20Contract = types.StringToAddress("0x1003")
	// ChildERC20PredicateContract is an address of child ERC20 predicate contract on the child chain
	ChildERC20PredicateContract = types.StringToAddress("0x1004")
	// SystemCaller is address of account, used for system calls to smart contracts
	SystemCaller = types.StringToAddress("0xffffFFFfFFffffffffffffffFfFFFfffFFFfFFfE")

	// NativeTransferPrecompile is an address of native transfer precompile
	NativeTransferPrecompile = types.StringToAddress("0x2020")
	// BLSAggSigsVerificationPrecompile is an address of BLS aggregated signatures verificatin precompile
	BLSAggSigsVerificationPrecompile = types.StringToAddress("0x2030")
	// ConsolePrecompile is and address of Hardhat console precompile
	ConsolePrecompile = types.StringToAddress("0x000000000000000000636F6e736F6c652e6c6f67")
)
