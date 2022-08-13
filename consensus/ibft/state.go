package ibft

import (
	"math"

	"github.com/0xPolygon/polygon-edge/types"
)

type ValidatorSet []types.Address

// CalcProposer calculates the address of the next proposer, from the validator set
func (v *ValidatorSet) CalcProposer(round uint64, lastProposer types.Address) types.Address {
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

// Add adds a new address to the validator set
func (v *ValidatorSet) Add(addr types.Address) {
	*v = append(*v, addr)
}

// Del removes an address from the validator set
func (v *ValidatorSet) Del(addr types.Address) {
	for indx, i := range *v {
		if i == addr {
			*v = append((*v)[:indx], (*v)[indx+1:]...)
		}
	}
}

// Len returns the size of the validator set
func (v *ValidatorSet) Len() int {
	return len(*v)
}

// Equal checks if 2 validator sets are equal
func (v *ValidatorSet) Equal(vv *ValidatorSet) bool {
	if len(*v) != len(*vv) {
		return false
	}

	for indx := range *v {
		if (*v)[indx] != (*vv)[indx] {
			return false
		}
	}

	return true
}

// Index returns the index of the passed in address in the validator set.
// Returns -1 if not found
func (v *ValidatorSet) Index(addr types.Address) int {
	for indx, i := range *v {
		if i == addr {
			return indx
		}
	}

	return -1
}

// Includes checks if the address is in the validator set
func (v *ValidatorSet) Includes(addr types.Address) bool {
	return v.Index(addr) != -1
}

// MaxFaultyNodes returns the maximum number of allowed faulty nodes (F), based on the current validator set
func (v *ValidatorSet) MaxFaultyNodes() int {
	// N -> number of nodes in IBFT
	// F -> number of faulty nodes
	//
	// N = 3F + 1
	// => F = (N - 1) / 3
	//
	// IBFT tolerates 1 failure with 4 nodes
	// 4 = 3 * 1 + 1
	// To tolerate 2 failures, IBFT requires 7 nodes
	// 7 = 3 * 2 + 1
	// It should always take the floor of the result
	return (len(*v) - 1) / 3
}

type QuorumImplementation func(ValidatorSet) int

//	LegacyQuorumSize returns the legacy quorum size for the given validator set
func LegacyQuorumSize(set ValidatorSet) int {
	// According to the IBFT spec, the number of valid messages
	// needs to be 2F + 1
	return 2*set.MaxFaultyNodes() + 1
}

// OptimalQuorumSize returns the optimal quorum size for the given validator set
func OptimalQuorumSize(set ValidatorSet) int {
	//	if the number of validators is less than 4,
	//	then the entire set is required
	if set.MaxFaultyNodes() == 0 {
		/*
			N: 1 -> Q: 1
			N: 2 -> Q: 2
			N: 3 -> Q: 3
		*/
		return set.Len()
	}

	// (quorum optimal)	Q = ceil(2/3 * N)
	return int(math.Ceil(2 * float64(set.Len()) / 3))
}
