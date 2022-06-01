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

func CalcQuorumSize(s ValidatorSet) int {
	//	if the number of validators is less than 4,
	//	then the entire set is required
	if s.MaxFaultyNodes() == 0 {
		/*
			N: 1 -> Q: 1
			N: 2 -> Q: 2
			N: 3 -> Q: 3
		*/
		return s.Len()
	}

	// (quorum optimal)	Q = ceil(2/3 * N)
	return int(math.Ceil(2 * float64(s.Len()) / 3))
}
