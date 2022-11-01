package polybft

import (
	"github.com/0xPolygon/polygon-edge/types"
)

// ValidatorSet is a wrapper interface around pbft.ValidatorSet and it holds current validator set
type ValidatorSet interface {
	CalcProposer(round uint64) types.Address
	Includes(address types.Address) bool
	Len() int

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
