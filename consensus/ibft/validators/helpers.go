package validators

import "math"

func CalcMaxFaultyNodes(s ValidatorSet) int {
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
