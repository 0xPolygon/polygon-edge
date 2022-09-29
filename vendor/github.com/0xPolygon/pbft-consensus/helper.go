package pbft

// QuorumSize calculates quorum size (namely the number of required messages of some type in order to proceed to the next state in PolyBFT state machine).
// It is calculated by formula:
// 2 * F + 1, where F denotes maximum count of faulty nodes in order to have Byzantine fault tollerant property satisfied.
func QuorumSize(nodesCount int) int {
	return 2*MaxFaultyNodes(nodesCount) + 1
}

// MaxFaultyNodes calculate max faulty nodes in order to have Byzantine-fault tollerant system.
// Formula explanation:
// N -> number of nodes in PBFT
// F -> number of faulty nodes
// N = 3 * F + 1 => F = (N - 1) / 3
//
// PBFT tolerates 1 failure with 4 nodes
// 4 = 3 * 1 + 1
// To tolerate 2 failures, PBFT requires 7 nodes
// 7 = 3 * 2 + 1
// It should always take the floor of the result
func MaxFaultyNodes(nodesCount int) int {
	if nodesCount <= 0 {
		return 0
	}

	return (nodesCount - 1) / 3
}

func TotalVotingPower(mp map[NodeID]uint64) uint64 {
	var totalVotingPower uint64
	for _, v := range mp {
		totalVotingPower += v
	}
	return totalVotingPower
}

func MaxFaultyVP(vp uint64) uint64 {
	if vp == 0 {
		return 0
	}
	return (vp - 1) / 3
}

func QuorumSizeVP(totalVP uint64) uint64 {
	return 2*MaxFaultyVP(totalVP) + 1
}
