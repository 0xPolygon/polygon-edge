package pbft

type ValidatorKeyMock string

func (k ValidatorKeyMock) NodeID() NodeID {
	return NodeID(k)
}

func (k ValidatorKeyMock) Sign(b []byte) ([]byte, error) {
	return b, nil
}

type TransportStub struct {
	Nodes      []*Pbft
	GossipFunc func(ft *TransportStub, msg *MessageReq) error
}

func (ft *TransportStub) Gossip(msg *MessageReq) error {
	if ft.GossipFunc != nil {
		return ft.GossipFunc(ft, msg)
	}

	for _, node := range ft.Nodes {
		if msg.From != node.GetValidatorId() {
			node.PushMessage(msg.Copy())
		}
	}
	return nil
}

func NewValStringStub(nodes []NodeID, votingPowerMap map[NodeID]uint64) *ValStringStub {
	return &ValStringStub{
		Nodes:          nodes,
		VotingPowerMap: votingPowerMap,
	}
}

type ValStringStub struct {
	Nodes          []NodeID
	VotingPowerMap map[NodeID]uint64
}

func (v *ValStringStub) CalcProposer(round uint64) NodeID {
	seed := uint64(0)

	offset := 0
	// add last proposer

	seed = uint64(offset) + round
	pick := seed % uint64(v.Len())

	return (v.Nodes)[pick]
}

func (v *ValStringStub) Index(id NodeID) int {
	for i, currentId := range v.Nodes {
		if currentId == id {
			return i
		}
	}

	return -1
}

func (v *ValStringStub) Includes(id NodeID) bool {
	for _, currentId := range v.Nodes {
		if currentId == id {
			return true
		}
	}
	return false
}

func (v *ValStringStub) Len() int {
	return len(v.Nodes)
}

func (v *ValStringStub) VotingPower() map[NodeID]uint64 {
	return v.VotingPowerMap
}

// CreateEqualVotingPowerMap is a helper function which creates map with same weight for every validator id in the provided slice
func CreateEqualVotingPowerMap(validatorIds []NodeID) map[NodeID]uint64 {
	weightedValidators := make(map[NodeID]uint64, len(validatorIds))
	for _, validatorId := range validatorIds {
		weightedValidators[validatorId] = 1
	}
	return weightedValidators
}
