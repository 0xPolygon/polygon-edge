package staking

import (
	"fmt"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"math"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	StakingSCAddress = types.StringToAddress("1001")
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
	storageIndexes.MinimumNumValidatorsIndex = big.NewInt(minimumNumValidator).Bytes()
	storageIndexes.MaximumNumValidatorsIndex = big.NewInt(maximumNumValidator).Bytes()

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

// StorageIndexes is a wrapper for different storage indexes that
// need to be modified
type StorageIndexes struct {
	ValidatorsIndex              []byte // []address
	ValidatorsArraySizeIndex     []byte // []address size
	AddressToIsValidatorIndex    []byte // mapping(address => bool)
	AddressToStakedAmountIndex   []byte // mapping(address => uint256)
	AddressToValidatorIndexIndex []byte // mapping(address => uint256)
	StakedAmountIndex            []byte // uint256
	MinimumNumValidatorsIndex    []byte // uint256
	MaximumNumValidatorsIndex    []byte // uint256
}

// Slot definitions for SC storage
var (
	validatorsSlot              = int64(0) // Slot 0
	addressToIsValidatorSlot    = int64(1) // Slot 1
	addressToStakedAmountSlot   = int64(2) // Slot 2
	addressToValidatorIndexSlot = int64(3) // Slot 3
	stakedAmountSlot            = int64(4) // Slot 4
	minimumNumValidator         = int64(5) // Slot 5
	maximumNumValidator         = int64(6) // Slot 6
)

const (
	DefaultStakedBalance = "0x8AC7230489E80000" // 10 ETH
	//nolint: lll
	StakingSCBytecode = "0x60806040526004361061007f5760003560e01c806350d68ed81161004e57806350d68ed81461017b578063ca1e7819146101a6578063f90ecacc146101d1578063facd743b1461020e576100ed565b80632367f6b5146100f25780632def66201461012f578063373d6132146101465780633a4b66f114610171576100ed565b366100ed576100a33373ffffffffffffffffffffffffffffffffffffffff1661024b565b156100e3576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004016100da90610f90565b60405180910390fd5b6100eb61025e565b005b600080fd5b3480156100fe57600080fd5b5061011960048036038101906101149190610cee565b61043e565b6040516101269190610fcb565b60405180910390f35b34801561013b57600080fd5b50610144610487565b005b34801561015257600080fd5b5061015b610572565b6040516101689190610fcb565b60405180910390f35b61017961057c565b005b34801561018757600080fd5b506101906105e5565b60405161019d9190610fb0565b60405180910390f35b3480156101b257600080fd5b506101bb6105f1565b6040516101c89190610ed3565b60405180910390f35b3480156101dd57600080fd5b506101f860048036038101906101f39190610d1b565b61067f565b6040516102059190610eb8565b60405180910390f35b34801561021a57600080fd5b5061023560048036038101906102309190610cee565b6106be565b6040516102429190610ef5565b60405180910390f35b600080823b905060008111915050919050565b600560009054906101000a900463ffffffff1663ffffffff16600080549050106102bd576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004016102b490610f50565b60405180910390fd5b34600460008282546102cf9190611030565b9250508190555034600260003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008282546103259190611030565b92505081905550600160003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060009054906101000a900460ff161580156103df5750670de0b6b3a76400006fffffffffffffffffffffffffffffffff16600260003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205410155b156103ee576103ed33610714565b5b3373ffffffffffffffffffffffffffffffffffffffff167f9e71bc8eea02a63969f509818f2dafb9254532904319f9dbda79b67bd34a5f3d346040516104349190610fcb565b60405180910390a2565b6000600260008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020549050919050565b6104a63373ffffffffffffffffffffffffffffffffffffffff1661024b565b156104e6576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004016104dd90610f90565b60405180910390fd5b6000600260003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205411610568576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040161055f90610f30565b60405180910390fd5b61057061081a565b565b6000600454905090565b61059b3373ffffffffffffffffffffffffffffffffffffffff1661024b565b156105db576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004016105d290610f90565b60405180910390fd5b6105e361025e565b565b670de0b6b3a764000081565b6060600080548060200260200160405190810160405280929190818152602001828054801561067557602002820191906000526020600020905b8160009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001906001019080831161062b575b5050505050905090565b6000818154811061068f57600080fd5b906000526020600020016000915054906101000a900473ffffffffffffffffffffffffffffffffffffffff1681565b6000600160008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060009054906101000a900460ff169050919050565b60018060008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060006101000a81548160ff021916908315150217905550600080549050600360008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055506000819080600181540180825580915050600190039060005260206000200160009091909190916101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff16021790555050565b600560049054906101000a900463ffffffff1663ffffffff1660008054905011610879576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040161087090610f10565b60405180910390fd5b6000600260003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020549050600160003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060009054906101000a900460ff16156109195761091833610a0f565b5b6000600260003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000208190555080600460008282546109709190611086565b925050819055503373ffffffffffffffffffffffffffffffffffffffff166108fc829081150290604051600060405180830381858888f193505050501580156109bd573d6000803e3d6000fd5b503373ffffffffffffffffffffffffffffffffffffffff167f0f5bb82176feb1b5e747e28471aa92156a04d9f3ab9f45f28e2d704232b93f7582604051610a049190610fcb565b60405180910390a250565b600080549050600360008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205410610a95576040517f08c379a0000000000000000000000000000000000000000000000000000000008152600401610a8c90610f70565b60405180910390fd5b6000600360008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054905060006001600080549050610aed9190611086565b9050808214610bdb576000808281548110610b0b57610b0a61117c565b5b9060005260206000200160009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1690508060008481548110610b4d57610b4c61117c565b5b9060005260206000200160006101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff16021790555082600360008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002081905550505b6000600160008573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060006101000a81548160ff0219169083151502179055506000600360008573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055506000805480610c8a57610c8961114d565b5b6001900381819060005260206000200160006101000a81549073ffffffffffffffffffffffffffffffffffffffff02191690559055505050565b600081359050610cd3816112ef565b92915050565b600081359050610ce881611306565b92915050565b600060208284031215610d0457610d036111ab565b5b6000610d1284828501610cc4565b91505092915050565b600060208284031215610d3157610d306111ab565b5b6000610d3f84828501610cd9565b91505092915050565b6000610d548383610d60565b60208301905092915050565b610d69816110ba565b82525050565b610d78816110ba565b82525050565b6000610d8982610ff6565b610d93818561100e565b9350610d9e83610fe6565b8060005b83811015610dcf578151610db68882610d48565b9750610dc183611001565b925050600181019050610da2565b5085935050505092915050565b610de5816110cc565b82525050565b6000610df860448361101f565b9150610e03826111b0565b606082019050919050565b6000610e1b601d8361101f565b9150610e2682611225565b602082019050919050565b6000610e3e60278361101f565b9150610e498261124e565b604082019050919050565b6000610e6160128361101f565b9150610e6c8261129d565b602082019050919050565b6000610e84601a8361101f565b9150610e8f826112c6565b602082019050919050565b610ea3816110d8565b82525050565b610eb281611114565b82525050565b6000602082019050610ecd6000830184610d6f565b92915050565b60006020820190508181036000830152610eed8184610d7e565b905092915050565b6000602082019050610f0a6000830184610ddc565b92915050565b60006020820190508181036000830152610f2981610deb565b9050919050565b60006020820190508181036000830152610f4981610e0e565b9050919050565b60006020820190508181036000830152610f6981610e31565b9050919050565b60006020820190508181036000830152610f8981610e54565b9050919050565b60006020820190508181036000830152610fa981610e77565b9050919050565b6000602082019050610fc56000830184610e9a565b92915050565b6000602082019050610fe06000830184610ea9565b92915050565b6000819050602082019050919050565b600081519050919050565b6000602082019050919050565b600082825260208201905092915050565b600082825260208201905092915050565b600061103b82611114565b915061104683611114565b9250827fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff0382111561107b5761107a61111e565b5b828201905092915050565b600061109182611114565b915061109c83611114565b9250828210156110af576110ae61111e565b5b828203905092915050565b60006110c5826110f4565b9050919050565b60008115159050919050565b60006fffffffffffffffffffffffffffffffff82169050919050565b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b6000819050919050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b7f4e487b7100000000000000000000000000000000000000000000000000000000600052603160045260246000fd5b7f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b600080fd5b7f4e756d626572206f662076616c696461746f72732063616e2774206265206c6560008201527f7373207468616e204d696e696d756d52657175697265644e756d56616c69646160208201527f746f727300000000000000000000000000000000000000000000000000000000604082015250565b7f4f6e6c79207374616b65722063616e2063616c6c2066756e6374696f6e000000600082015250565b7f56616c696461746f72207365742068617320726561636865642066756c6c206360008201527f6170616369747900000000000000000000000000000000000000000000000000602082015250565b7f696e646578206f7574206f662072616e67650000000000000000000000000000600082015250565b7f4f6e6c7920454f412063616e2063616c6c2066756e6374696f6e000000000000600082015250565b6112f8816110ba565b811461130357600080fd5b50565b61130f81611114565b811461131a57600080fd5b5056fea2646970667358221220a8138d18f101e6f82542c730da59e80cf72514739559efba0f22714c63d74fa164736f6c63430008070033"
)

// PredeployStakingSC is a helper method for setting up the staking smart contract account,
// using the passed in validators as prestaked validators
func PredeployStakingSC(
	validators []types.Address,
	minValidatorCount uint,
	maxValidatorCount uint,
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

	if minValidatorCount > maxValidatorCount {
		return nil, fmt.Errorf("minimum number of validator can not be greater than maximum number of validator")
	}

	if maxValidatorCount > math.MaxUint32 {
		maxValidatorCount = math.MaxUint32
	}

	// Generate the empty account storage map
	storageMap := make(map[types.Hash]types.Hash)
	bigTrueValue := big.NewInt(1)
	stakedAmount := big.NewInt(0)
	minNumValidators := big.NewInt(int64(minValidatorCount))
	maxNumValidators := big.NewInt(int64(maxValidatorCount))

	for indx, validator := range validators {
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

		// Set the value for the minimum number of validators
		storageMap[types.BytesToHash(storageIndexes.MinimumNumValidatorsIndex)] =
			types.BytesToHash(minNumValidators.Bytes())

		// Set the value for the maximum number of validators
		storageMap[types.BytesToHash(storageIndexes.MaximumNumValidatorsIndex)] =
			types.BytesToHash(maxNumValidators.Bytes())
	}

	// Save the storage map
	stakingAccount.Storage = storageMap

	// Set the Staking SC balance to numValidators * defaultStakedBalance
	stakingAccount.Balance = stakedAmount

	return stakingAccount, nil
}
