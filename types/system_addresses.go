package types

var (
	// ValidatorSetContract is an address of validator set contract deployed to child chain
	ValidatorSetContract = StringToAddress("0x101")
	// BLSContract is an address of BLS contract on the child chain
	BLSContract = StringToAddress("0x102")
	// MerkleContract is an address of Merkle contract on the child chain
	MerkleContract = StringToAddress("0x103")
	// StateReceiverContract is an address of bridge contract on the child chain
	StateReceiverContract = StringToAddress("0x1001")
	// NativeTokenContract is an address of bridge contract (used for transferring native tokens on child chain)
	NativeTokenContract = StringToAddress("0x1010")
	// SystemCaller is address of account, used for system calls to smart contracts
	SystemCaller = StringToAddress("0xffffFFFfFFffffffffffffffFfFFFfffFFFfFFfE")
	// L2StateSender is an address of bridge contract to the rootchain
	L2StateSenderContract = StringToAddress("0x1002")

	// NativeTransferPrecompile is an address of native transfer precompile
	NativeTransferPrecompile = StringToAddress("0x2020")
	// BLSAggSigsVerificationPrecompile is an address of BLS aggregated signatures verificatin precompile
	BLSAggSigsVerificationPrecompile = StringToAddress("0x2030")
	// ConsolePrecompile is and address of Hardhat console precompile
	ConsolePrecompile = StringToAddress("0x000000000000000000636F6e736F6c652e6c6f67")
)
