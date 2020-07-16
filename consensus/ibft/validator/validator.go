package validator

import (
	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/0xPolygon/minimal/types"
)

func New(addr types.Address) ibft.Validator {
	return &defaultValidator{
		address: addr,
	}
}

func NewSet(addrs []types.Address, policy ibft.ProposerPolicy) ibft.ValidatorSet {
	return newDefaultSet(addrs, policy)
}

func ExtractValidators(extraData []byte) []types.Address {
	// get the validator addresses
	addrs := make([]types.Address, (len(extraData) / types.AddressLength))
	for i := 0; i < len(addrs); i++ {
		copy(addrs[i][:], extraData[i*types.AddressLength:])
	}

	return addrs
}

// Check whether the extraData is presented in prescribed form
func ValidExtraData(extraData []byte) bool {
	return len(extraData)%types.AddressLength == 0
}
