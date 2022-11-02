package polybft

import (
	"github.com/0xPolygon/polygon-edge/types"
)

// ValidatorSet interface of the current validator set
type ValidatorSet interface {

	// CalcProposer calculates next proposer based on the passed round
	CalcProposer(round uint64) types.Address

	// Includes checks if the passed address in included in the current validator set
	Includes(address types.Address) bool

	// Len returns the size of the validator set
	Len() int

	// Accounts returns the list of the ValidatorAccount
	Accounts() AccountSet
}

type validatorSet struct {
	// last proposer of a block
	last types.Address

	// current list of validators (slice of (Address, BlsPublicKey) pairs)
	validators AccountSet
}

func newValidatorSet(lastProposer types.Address, validators AccountSet) *validatorSet {
	return &validatorSet{validators: validators, last: lastProposer}
}

func (v validatorSet) Accounts() AccountSet {
	return v.validators
}

func (v validatorSet) CalcProposer(round uint64) types.Address {
	var seed uint64
	if v.last == (types.Address{}) {
		seed = round
	} else {
		offset := 0
		if indx := v.validators.Index(v.last); indx != -1 {
			offset = indx
		}

		seed = uint64(offset) + round + 1
	}

	pick := seed % uint64(v.validators.Len())

	return v.validators[pick].Address
}

func (v validatorSet) Includes(address types.Address) bool {
	return v.validators.ContainsAddress(address)
}

func (v validatorSet) Len() int {
	return v.validators.Len()
}
