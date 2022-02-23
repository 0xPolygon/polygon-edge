package staking

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	MinValidatorCount = uint(1)
	MaxValidatorCount = uint(math.MaxUint32)
)

var (
	ErrInvalidValidatorNumber = errors.New("minimum number of validator can not be greater than maximum number of validator")
)

// PadLeftOrTrim left-pads the passed in byte array to the specified size,
// or trims the array if it exceeds the passed in size
func PadLeftOrTrim(bb []byte, size int) []byte {
	l := len(bb)
	if l == size {
		return bb
	}

	if l > size {
		return bb[l-size:]
	}

	tmp := make([]byte, size)
	copy(tmp[size-l:], bb)

	return tmp
}

// getAddressMapping returns the key for the SC storage mapping (address => something)
//
// More information:
// https://docs.soliditylang.org/en/latest/internals/layout_in_storage.html
func getAddressMapping(address types.Address, slot int64) []byte {
	bigSlot := big.NewInt(slot)

	finalSlice := append(
		PadLeftOrTrim(address.Bytes(), 32),
		PadLeftOrTrim(bigSlot.Bytes(), 32)...,
	)
	keccakValue := keccak.Keccak256(nil, finalSlice)

	return keccakValue
}

// getIndexWithOffset is a helper method for adding an offset to the already found keccak hash
func getIndexWithOffset(keccakHash []byte, offset int64) []byte {
	bigOffset := big.NewInt(offset)
	bigKeccak := big.NewInt(0).SetBytes(keccakHash)

	bigKeccak.Add(bigKeccak, bigOffset)

	return bigKeccak.Bytes()
}

// getStorageIndexes is a helper function for getting the correct indexes
// of the storage slots which need to be modified during bootstrap.
//
// It is SC dependant, and based on the SC located at:
// https://github.com/0xPolygon/staking-contracts/
func getStorageIndexes(address types.Address, index int64) *StorageIndexes {
	storageIndexes := StorageIndexes{}

	// Get the indexes for the mappings
	// The index for the mapping is retrieved with:
	// keccak(address . slot)
	// . stands for concatenation (basically appending the bytes)
	storageIndexes.AddressToIsValidatorIndex = getAddressMapping(address, addressToIsValidatorSlot)
	storageIndexes.AddressToStakedAmountIndex = getAddressMapping(address, addressToStakedAmountSlot)
	storageIndexes.AddressToValidatorIndexIndex = getAddressMapping(address, addressToValidatorIndexSlot)

	// Get the indexes for _validators, _stakedAmount, _minNumValidators, _maxNumValidators
	// Index for regular types is calculated as just the regular slot
	storageIndexes.StakedAmountIndex = big.NewInt(stakedAmountSlot).Bytes()
	storageIndexes.MinAndMaxNumValidatorsIndex = big.NewInt(minAndMaxNumValidator).Bytes()

	// Index for array types is calculated as keccak(slot) + index
	// The slot for the dynamic arrays that's put in the keccak needs to be in hex form (padded 64 chars)
	storageIndexes.ValidatorsIndex = getIndexWithOffset(
		keccak.Keccak256(nil, PadLeftOrTrim(big.NewInt(validatorsSlot).Bytes(), 32)),
		index,
	)

	// For any dynamic array in Solidity, the size of the actual array should be
	// located on slot x
	storageIndexes.ValidatorsArraySizeIndex = []byte{byte(validatorsSlot)}

	return &storageIndexes
}

// PredeployParams contains the values used to predeploy the PoS staking contract
type PredeployParams struct {
	MinValidatorCount uint
	MaxValidatorCount uint
}

// StorageIndexes is a wrapper for different storage indexes that
// need to be modified
type StorageIndexes struct {
	ValidatorsIndex              []byte // []address
	ValidatorsArraySizeIndex     []byte // []address size
	AddressToIsValidatorIndex    []byte // mapping(address => bool)
	AddressToStakedAmountIndex   []byte // mapping(address => uint256)
	AddressToValidatorIndexIndex []byte // mapping(address => uint256)
	StakedAmountIndex            []byte // uint256
	MinAndMaxNumValidatorsIndex  []byte // uint256
}

// Slot definitions for SC storage
var (
	validatorsSlot              = int64(0) // Slot 0
	addressToIsValidatorSlot    = int64(1) // Slot 1
	addressToStakedAmountSlot   = int64(2) // Slot 2
	addressToValidatorIndexSlot = int64(3) // Slot 3
	stakedAmountSlot            = int64(4) // Slot 4
	minAndMaxNumValidator       = int64(5) // Slot 5
)

const (
	DefaultStakedBalance = "0x8AC7230489E80000" // 10 ETH
	//nolint: lll
	StakingSCBytecode = "0x6080604052600436106100955760003560e01c8063714ff42511610059578063714ff425146101bc578063ca1e7819146101e7578063e804fbf614610212578063f90ecacc1461023d578063facd743b1461027a57610103565b80632367f6b5146101085780632def662014610145578063373d61321461015c5780633a4b66f11461018757806350d68ed81461019157610103565b36610103576100b93373ffffffffffffffffffffffffffffffffffffffff166102b7565b156100f9576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004016100f09061103f565b60405180910390fd5b6101016102ca565b005b600080fd5b34801561011457600080fd5b5061012f600480360381019061012a9190610d8e565b6104aa565b60405161013c919061107a565b60405180910390f35b34801561015157600080fd5b5061015a6104f3565b005b34801561016857600080fd5b506101716105de565b60405161017e919061107a565b60405180910390f35b61018f6105e8565b005b34801561019d57600080fd5b506101a6610651565b6040516101b3919061105f565b60405180910390f35b3480156101c857600080fd5b506101d161065d565b6040516101de9190611095565b60405180910390f35b3480156101f357600080fd5b506101fc610677565b6040516102099190610f82565b60405180910390f35b34801561021e57600080fd5b50610227610705565b6040516102349190611095565b60405180910390f35b34801561024957600080fd5b50610264600480360381019061025f9190610dbb565b61071f565b6040516102719190610f67565b60405180910390f35b34801561028657600080fd5b506102a1600480360381019061029c9190610d8e565b61075e565b6040516102ae9190610fa4565b60405180910390f35b600080823b905060008111915050919050565b600560049054906101000a900463ffffffff1663ffffffff1660008054905010610329576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040161032090610fff565b60405180910390fd5b346004600082825461033b91906110fa565b9250508190555034600260003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020600082825461039191906110fa565b92505081905550600160003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060009054906101000a900460ff1615801561044b5750670de0b6b3a76400006fffffffffffffffffffffffffffffffff16600260003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205410155b1561045a57610459336107b4565b5b3373ffffffffffffffffffffffffffffffffffffffff167f9e71bc8eea02a63969f509818f2dafb9254532904319f9dbda79b67bd34a5f3d346040516104a0919061107a565b60405180910390a2565b6000600260008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020549050919050565b6105123373ffffffffffffffffffffffffffffffffffffffff166102b7565b15610552576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004016105499061103f565b60405180910390fd5b6000600260003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054116105d4576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004016105cb90610fdf565b60405180910390fd5b6105dc6108ba565b565b6000600454905090565b6106073373ffffffffffffffffffffffffffffffffffffffff166102b7565b15610647576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040161063e9061103f565b60405180910390fd5b61064f6102ca565b565b670de0b6b3a764000081565b6000600560009054906101000a900463ffffffff16905090565b606060008054806020026020016040519081016040528092919081815260200182805480156106fb57602002820191906000526020600020905b8160009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190600101908083116106b1575b5050505050905090565b6000600560049054906101000a900463ffffffff16905090565b6000818154811061072f57600080fd5b906000526020600020016000915054906101000a900473ffffffffffffffffffffffffffffffffffffffff1681565b6000600160008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060009054906101000a900460ff169050919050565b60018060008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060006101000a81548160ff021916908315150217905550600080549050600360008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055506000819080600181540180825580915050600190039060005260206000200160009091909190916101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff16021790555050565b600560009054906101000a900463ffffffff1663ffffffff1660008054905011610919576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040161091090610fbf565b60405180910390fd5b6000600260003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020549050600160003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060009054906101000a900460ff16156109b9576109b833610aaf565b5b6000600260003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055508060046000828254610a109190611150565b925050819055503373ffffffffffffffffffffffffffffffffffffffff166108fc829081150290604051600060405180830381858888f19350505050158015610a5d573d6000803e3d6000fd5b503373ffffffffffffffffffffffffffffffffffffffff167f0f5bb82176feb1b5e747e28471aa92156a04d9f3ab9f45f28e2d704232b93f7582604051610aa4919061107a565b60405180910390a250565b600080549050600360008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205410610b35576040517f08c379a0000000000000000000000000000000000000000000000000000000008152600401610b2c9061101f565b60405180910390fd5b6000600360008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054905060006001600080549050610b8d9190611150565b9050808214610c7b576000808281548110610bab57610baa611256565b5b9060005260206000200160009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1690508060008481548110610bed57610bec611256565b5b9060005260206000200160006101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff16021790555082600360008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002081905550505b6000600160008573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060006101000a81548160ff0219169083151502179055506000600360008573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055506000805480610d2a57610d29611227565b5b6001900381819060005260206000200160006101000a81549073ffffffffffffffffffffffffffffffffffffffff02191690559055505050565b600081359050610d73816113c9565b92915050565b600081359050610d88816113e0565b92915050565b600060208284031215610da457610da3611285565b5b6000610db284828501610d64565b91505092915050565b600060208284031215610dd157610dd0611285565b5b6000610ddf84828501610d79565b91505092915050565b6000610df48383610e00565b60208301905092915050565b610e0981611184565b82525050565b610e1881611184565b82525050565b6000610e29826110c0565b610e3381856110d8565b9350610e3e836110b0565b8060005b83811015610e6f578151610e568882610de8565b9750610e61836110cb565b925050600181019050610e42565b5085935050505092915050565b610e8581611196565b82525050565b6000610e986044836110e9565b9150610ea38261128a565b606082019050919050565b6000610ebb601d836110e9565b9150610ec6826112ff565b602082019050919050565b6000610ede6027836110e9565b9150610ee982611328565b604082019050919050565b6000610f016012836110e9565b9150610f0c82611377565b602082019050919050565b6000610f24601a836110e9565b9150610f2f826113a0565b602082019050919050565b610f43816111a2565b82525050565b610f52816111de565b82525050565b610f61816111e8565b82525050565b6000602082019050610f7c6000830184610e0f565b92915050565b60006020820190508181036000830152610f9c8184610e1e565b905092915050565b6000602082019050610fb96000830184610e7c565b92915050565b60006020820190508181036000830152610fd881610e8b565b9050919050565b60006020820190508181036000830152610ff881610eae565b9050919050565b6000602082019050818103600083015261101881610ed1565b9050919050565b6000602082019050818103600083015261103881610ef4565b9050919050565b6000602082019050818103600083015261105881610f17565b9050919050565b60006020820190506110746000830184610f3a565b92915050565b600060208201905061108f6000830184610f49565b92915050565b60006020820190506110aa6000830184610f58565b92915050565b6000819050602082019050919050565b600081519050919050565b6000602082019050919050565b600082825260208201905092915050565b600082825260208201905092915050565b6000611105826111de565b9150611110836111de565b9250827fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff03821115611145576111446111f8565b5b828201905092915050565b600061115b826111de565b9150611166836111de565b925082821015611179576111786111f8565b5b828203905092915050565b600061118f826111be565b9050919050565b60008115159050919050565b60006fffffffffffffffffffffffffffffffff82169050919050565b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b6000819050919050565b600063ffffffff82169050919050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b7f4e487b7100000000000000000000000000000000000000000000000000000000600052603160045260246000fd5b7f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b600080fd5b7f4e756d626572206f662076616c696461746f72732063616e2774206265206c6560008201527f7373207468616e204d696e696d756d52657175697265644e756d56616c69646160208201527f746f727300000000000000000000000000000000000000000000000000000000604082015250565b7f4f6e6c79207374616b65722063616e2063616c6c2066756e6374696f6e000000600082015250565b7f56616c696461746f72207365742068617320726561636865642066756c6c206360008201527f6170616369747900000000000000000000000000000000000000000000000000602082015250565b7f696e646578206f7574206f662072616e67650000000000000000000000000000600082015250565b7f4f6e6c7920454f412063616e2063616c6c2066756e6374696f6e000000000000600082015250565b6113d281611184565b81146113dd57600080fd5b50565b6113e9816111de565b81146113f457600080fd5b5056fea26469706673582212203250498e183ccb32ad1108771218728346f5ba3328c9c18fcfcef208abfedc9764736f6c63430008070033"
)

// PredeployStakingSC is a helper method for setting up the staking smart contract account,
// using the passed in validators as prestaked validators
func PredeployStakingSC(
	validators []types.Address,
	params PredeployParams,
) (*chain.GenesisAccount, error) {
	// Set the code for the staking smart contract
	// Code retrieved from https://github.com/0xPolygon/staking-contracts
	scHex, _ := hex.DecodeHex(StakingSCBytecode)
	stakingAccount := &chain.GenesisAccount{
		Code: scHex,
	}

	// Parse the default staked balance value into *big.Int
	val := DefaultStakedBalance
	bigDefaultStakedBalance, err := types.ParseUint256orHex(&val)

	if err != nil {
		return nil, fmt.Errorf("unable to generate DefaultStatkedBalance, %w", err)
	}

	if params.MinValidatorCount > params.MaxValidatorCount {
		return nil, ErrInvalidValidatorNumber
	}

	if params.MaxValidatorCount > math.MaxUint32 {
		params.MaxValidatorCount = math.MaxUint32
	}

	// Generate the empty account storage map
	storageMap := make(map[types.Hash]types.Hash)
	bigTrueValue := big.NewInt(1)
	stakedAmount := big.NewInt(0)
	minNumValidators := big.NewInt(int64(params.MinValidatorCount))
	maxNumValidators := big.NewInt(int64(params.MaxValidatorCount))

	for indx, validator := range validators {
		if int64(indx) == maxNumValidators.Int64() {
			break
		}
		// Update the total staked amount
		stakedAmount.Add(stakedAmount, bigDefaultStakedBalance)

		// Get the storage indexes
		storageIndexes := getStorageIndexes(validator, int64(indx))

		// Set the value for the validators array
		storageMap[types.BytesToHash(storageIndexes.ValidatorsIndex)] =
			types.BytesToHash(
				validator.Bytes(),
			)

		// Set the value for the address -> validator array index mapping
		storageMap[types.BytesToHash(storageIndexes.AddressToIsValidatorIndex)] =
			types.BytesToHash(bigTrueValue.Bytes())

		// Set the value for the address -> staked amount mapping
		storageMap[types.BytesToHash(storageIndexes.AddressToStakedAmountIndex)] =
			types.StringToHash(hex.EncodeBig(bigDefaultStakedBalance))

		// Set the value for the address -> validator index mapping
		storageMap[types.BytesToHash(storageIndexes.AddressToValidatorIndexIndex)] =
			types.StringToHash(hex.EncodeUint64(uint64(indx)))

		// Set the value for the total staked amount
		storageMap[types.BytesToHash(storageIndexes.StakedAmountIndex)] =
			types.BytesToHash(stakedAmount.Bytes())

		// Set the value for the size of the validators array
		storageMap[types.BytesToHash(storageIndexes.ValidatorsArraySizeIndex)] =
			types.StringToHash(hex.EncodeUint64(uint64(indx + 1)))

		// Set the value for the minimum and maximum number of validators
		min := PadLeftOrTrim(minNumValidators.Bytes(), 4)
		max := PadLeftOrTrim(maxNumValidators.Bytes(), 4)
		storageMap[types.BytesToHash(storageIndexes.MinAndMaxNumValidatorsIndex)] = types.BytesToHash(
			append(
				max,
				min...,
			),
		)
	}

	// Save the storage map
	stakingAccount.Storage = storageMap

	// Set the Staking SC balance to numValidators * defaultStakedBalance
	stakingAccount.Balance = stakedAmount

	return stakingAccount, nil
}
