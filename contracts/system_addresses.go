package contracts

import "github.com/0xPolygon/polygon-edge/types"

var (
	// ValidatorSetContract is an address of validator set proxy contract deployed to child chain
	ValidatorSetContract = types.StringToAddress("0x101")
	// ValidatorSetContractV1 is an address of validator set implementation contract deployed to child chain
	ValidatorSetContractV1 = types.StringToAddress("0x1011")
	// BLSContract is an address of BLS proxy contract on the child chain
	BLSContract = types.StringToAddress("0x102")
	// BLSContractV1 is an address of BLS contract on the child chain
	BLSContractV1 = types.StringToAddress("0x1021")
	// MerkleContract is an address of Merkle proxy contract on the child chain
	MerkleContract = types.StringToAddress("0x103")
	// MerkleContractV1 is an address of Merkle contract on the child chain
	MerkleContractV1 = types.StringToAddress("0x1031")
	// RewardTokenContract is an address of reward token proxy on child chain
	RewardTokenContract = types.StringToAddress("0x104")
	// RewardTokenContractV1 is an address of reward token on child chain
	RewardTokenContractV1 = types.StringToAddress("0x1041")
	// RewardPoolContract is an address of RewardPoolContract proxy contract on the child chain
	RewardPoolContract = types.StringToAddress("0x105")
	// RewardPoolContractV1 is an address of RewardPoolContract contract on the child chain
	RewardPoolContractV1 = types.StringToAddress("0x1051")
	// DefaultBurnContract is an address of eip1559 default proxy contract
	DefaultBurnContract = types.StringToAddress("0x106")
	// StateReceiverContract is an address of bridge proxy contract on the child chain
	StateReceiverContract = types.StringToAddress("0x1001")
	// StateReceiverContractV1 is an address of bridge implementation contract on the child chain
	StateReceiverContractV1 = types.StringToAddress("0x10011")
	// NativeERC20TokenContract is an address of bridge proxy contract
	// (used for transferring ERC20 native tokens on child chain)
	NativeERC20TokenContract = types.StringToAddress("0x1010")
	// NativeERC20TokenContractV1 is an address of bridge contract
	// (used for transferring ERC20 native tokens on child chain)
	NativeERC20TokenContractV1 = types.StringToAddress("0x10101")
	// L2StateSenderContract is an address of bridge proxy contract to the rootchain
	L2StateSenderContract = types.StringToAddress("0x1002")
	// L2StateSenderContractV1 is an address of bridge contract to the rootchain
	L2StateSenderContractV1 = types.StringToAddress("0x10021")

	// ChildERC20Contract is an address of bridgable ERC20 token contract on the child chain
	ChildERC20Contract = types.StringToAddress("0x1003")
	// ChildERC20PredicateContract is an address of child ERC20 proxy predicate contract on the child chain
	ChildERC20PredicateContract = types.StringToAddress("0x1004")
	// ChildERC20PredicateContractV1 is an address of child ERC20 predicate contract on the child chain
	ChildERC20PredicateContractV1 = types.StringToAddress("0x10041")
	// ChildERC721Contract is an address of bridgable ERC721 token contract on the child chain
	ChildERC721Contract = types.StringToAddress("0x1005")
	// ChildERC721PredicateContract is an address of child ERC721 proxy predicate contract on the child chain
	ChildERC721PredicateContract = types.StringToAddress("0x1006")
	// ChildERC721PredicateContractV1 is an address of child ERC721 predicate contract on the child chain
	ChildERC721PredicateContractV1 = types.StringToAddress("0x10061")
	// ChildERC1155Contract is an address of bridgable ERC1155 token contract on the child chain
	ChildERC1155Contract = types.StringToAddress("0x1007")
	// ChildERC1155PredicateContract is an address of child ERC1155 proxy predicate contract on the child chain
	ChildERC1155PredicateContract = types.StringToAddress("0x1008")
	// ChildERC1155PredicateContractV1 is an address of child ERC1155 predicate contract on the child chain
	ChildERC1155PredicateContractV1 = types.StringToAddress("0x10081")
	// RootMintableERC20PredicateContract is an address of mintable ERC20 proxy predicate on the child chain
	RootMintableERC20PredicateContract = types.StringToAddress("0x1009")
	// RootMintableERC20PredicateContractV1 is an address of mintable ERC20 predicate on the child chain
	RootMintableERC20PredicateContractV1 = types.StringToAddress("0x10091")
	// RootMintableERC721PredicateContract is an address of mintable ERC721 proxy predicate on the child chain
	RootMintableERC721PredicateContract = types.StringToAddress("0x100a")
	// RootMintableERC721PredicateContractV1 is an address of mintable ERC721 predicate on the child chain
	RootMintableERC721PredicateContractV1 = types.StringToAddress("0x100a1")
	// RootMintableERC1155PredicateContract is an address of mintable ERC1155 proxy predicate on the child chain
	RootMintableERC1155PredicateContract = types.StringToAddress("0x100b")
	// RootMintableERC1155PredicateContractV1 is an address of mintable ERC1155 predicate on the child chain
	RootMintableERC1155PredicateContractV1 = types.StringToAddress("0x100b1")

	// SystemCaller is address of account, used for system calls to smart contracts
	SystemCaller = types.StringToAddress("0xffffFFFfFFffffffffffffffFfFFFfffFFFfFFfE")

	// NativeTransferPrecompile is an address of native transfer precompile
	NativeTransferPrecompile = types.StringToAddress("0x2020")
	// BLSAggSigsVerificationPrecompile is an address of BLS aggregated signatures verificatin precompile
	BLSAggSigsVerificationPrecompile = types.StringToAddress("0x2030")
	// ConsolePrecompile is and address of Hardhat console precompile
	ConsolePrecompile = types.StringToAddress("0x000000000000000000636F6e736F6c652e6c6f67")
	// AllowListContractsAddr is the address of the contract deployer allow list
	AllowListContractsAddr = types.StringToAddress("0x0200000000000000000000000000000000000000")
	// BlockListContractsAddr is the address of the contract deployer block list
	BlockListContractsAddr = types.StringToAddress("0x0300000000000000000000000000000000000000")
	// AllowListTransactionsAddr is the address of the transactions allow list
	AllowListTransactionsAddr = types.StringToAddress("0x0200000000000000000000000000000000000002")
	// BlockListTransactionsAddr is the address of the transactions block list
	BlockListTransactionsAddr = types.StringToAddress("0x0300000000000000000000000000000000000002")
	// AllowListBridgeAddr is the address of the bridge allow list
	AllowListBridgeAddr = types.StringToAddress("0x0200000000000000000000000000000000000004")
	// BlockListBridgeAddr is the address of the bridge block list
	BlockListBridgeAddr = types.StringToAddress("0x0300000000000000000000000000000000000004")
)

// GetProxyImplementationMapping retrieves the addresses of proxy contracts that should be deployed unconditionally
func GetProxyImplementationMapping() map[types.Address]types.Address {
	return map[types.Address]types.Address{
		StateReceiverContract:                StateReceiverContractV1,
		BLSContract:                          BLSContractV1,
		MerkleContract:                       MerkleContractV1,
		L2StateSenderContract:                L2StateSenderContractV1,
		ValidatorSetContract:                 ValidatorSetContractV1,
		RewardPoolContract:                   RewardPoolContractV1,
		NativeERC20TokenContract:             NativeERC20TokenContractV1,
		ChildERC20PredicateContract:          ChildERC20PredicateContractV1,
		ChildERC721PredicateContract:         ChildERC721PredicateContractV1,
		ChildERC1155PredicateContract:        ChildERC1155PredicateContractV1,
		RootMintableERC20PredicateContract:   RootMintableERC20PredicateContractV1,
		RootMintableERC721PredicateContract:  RootMintableERC721PredicateContractV1,
		RootMintableERC1155PredicateContract: RootMintableERC1155PredicateContractV1,
	}
}
