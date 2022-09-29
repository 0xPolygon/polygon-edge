package polybft

import (
	"github.com/0xPolygon/pbft-consensus"
	"github.com/0xPolygon/polygon-edge/types"
)

// ValidatorSet is a wrapper interface around pbft.ValidatorSet and it holds current validator set
type ValidatorSet interface {
	pbft.ValidatorSet

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

func (v validatorSet) CalcProposer(round uint64) pbft.NodeID {
	seed := uint64(0)
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
	return pbft.NodeID(v.validators[pick].Address.String())
}

func (v validatorSet) Includes(id pbft.NodeID) bool {
	return v.validators.ContainsNodeID(id)
}

func (v validatorSet) Len() int {
	return v.validators.Len()
}
