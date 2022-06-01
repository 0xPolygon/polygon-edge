package validators

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type ECDSAValidatorSet []types.Address

// Len returns the size of the validator set
func (v *ECDSAValidatorSet) Len() int {
	return len(*v)
}

// Equal checks if 2 validator sets are equal
func (v *ECDSAValidatorSet) Equal(tv ValidatorSet) bool {
	target, ok := tv.(*ECDSAValidatorSet)
	if !ok {
		return false
	}

	if len(*v) != len(*target) {
		return false
	}

	for indx := range *v {
		if (*v)[indx] != (*target)[indx] {
			return false
		}
	}

	return true
}

// Copy returns a clone of ECDSAValidatorSet
func (v *ECDSAValidatorSet) Copy() ValidatorSet {
	addrs := make([]types.Address, v.Len())
	copy(addrs, *v)

	clone := ECDSAValidatorSet(addrs)

	return &clone
}

//
func (v *ECDSAValidatorSet) GetAddress(index int) types.Address {
	if index < v.Len() {
		return (*v)[index]
	}

	return types.ZeroAddress
}

// Index returns the index of the passed in address in the validator set.
// Returns -1 if not found
func (v *ECDSAValidatorSet) Index(target types.Address) int {
	for index, addr := range *v {
		if addr == target {
			return index
		}
	}

	return -1
}

// Includes checks if the address is in the validator set
func (v *ECDSAValidatorSet) Includes(addr types.Address) bool {
	return v.Index(addr) != -1
}

// CalcProposer calculates the address of the next proposer, from the validator set
func (v *ECDSAValidatorSet) CalcProposer(round uint64, lastProposer types.Address) types.Address {
	var seed uint64

	if lastProposer == types.ZeroAddress {
		seed = round
	} else {
		offset := 0
		if indx := v.Index(lastProposer); indx != -1 {
			offset = indx
		}

		seed = uint64(offset) + round + 1
	}

	pick := seed % uint64(v.Len())

	return (*v)[pick]
}

// MaxFaultyNodes returns the maximum number of allowed faulty nodes (F), based on the current validator set
func (v *ECDSAValidatorSet) MaxFaultyNodes() int {
	return CalcMaxFaultyNodes(v)
}

// QuorumSize returns the number of required messages for consensus
func (v *ECDSAValidatorSet) QuorumSize() int {
	return CalcQuorumSize(v)
}

// Add adds a new address to the validator set
func (v *ECDSAValidatorSet) Add(addr types.Address) {
	*v = append(*v, addr)
}

// Del removes an address from the validator set
func (v *ECDSAValidatorSet) Del(target types.Address) {
	for indx, addr := range *v {
		if addr == target {
			*v = append((*v)[:indx], (*v)[indx+1:]...)
		}
	}
}

func (v *ECDSAValidatorSet) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	if len(*v) == 0 {
		return ar.NewNullArray()
	}

	valSet := ar.NewArray()

	for _, addr := range *v {
		addr := addr

		if len(addr) == 0 {
			valSet.Set(ar.NewNull())
		} else {
			valSet.Set(ar.NewBytes(addr[:]))
		}
	}

	return valSet
}

func (v *ECDSAValidatorSet) UnmarshalRLPFrom(p *fastrlp.Parser, vv *fastrlp.Value) error {
	vals, err := vv.GetElems()
	if err != nil {
		return fmt.Errorf("mismatch of RLP type for CommittedSeal, expected list but found %s", vv.Type())
	}

	valSet := make([]types.Address, len(vals))

	for index, val := range vals {
		raw := make([]byte, 0)
		if raw, err = val.GetBytes(raw); err != nil {
			return err
		}

		valSet[index] = types.BytesToAddress(raw)
	}

	(*v) = ECDSAValidatorSet(valSet)

	return nil
}
