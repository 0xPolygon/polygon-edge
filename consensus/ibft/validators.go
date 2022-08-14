package ibft

import (
	"math"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
)

func CalcMaxFaultyNodes(s validators.Validators) int {
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
	return (s.Len() - 1) / 3
}

type QuorumImplementation func(validators.Validators) int

// LegacyQuorumSize returns the legacy quorum size for the given validator set
func LegacyQuorumSize(set validators.Validators) int {
	// According to the IBFT spec, the number of valid messages
	// needs to be 2F + 1
	return 2*CalcMaxFaultyNodes(set) + 1
}

// OptimalQuorumSize returns the optimal quorum size for the given validator set
func OptimalQuorumSize(set validators.Validators) int {
	//	if the number of validators is less than 4,
	//	then the entire set is required
	if CalcMaxFaultyNodes(set) == 0 {
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

func CalcProposer(
	validators validators.Validators,
	round uint64,
	lastProposer types.Address,
) validators.Validator {
	var seed uint64

	if lastProposer == types.ZeroAddress {
		seed = round
	} else {
		offset := int64(0)

		if index := validators.Index(lastProposer); index != -1 {
			offset = index
		}

		seed = uint64(offset) + round + 1
	}

	pick := seed % uint64(validators.Len())

	return validators.At(pick)
}
