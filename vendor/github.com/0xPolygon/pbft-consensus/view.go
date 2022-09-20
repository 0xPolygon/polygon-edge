package pbft

import "fmt"

type View struct {
	// round is the current round/height being finalized
	Round uint64 `json:"round"`

	// Sequence is a sequence number inside the round
	Sequence uint64 `json:"sequence"`
}

// ViewMsg is the constructor of View
func ViewMsg(sequence, round uint64) *View {
	return &View{
		Round:    round,
		Sequence: sequence,
	}
}

func (v *View) Copy() *View {
	vv := new(View)
	*vv = *v
	return vv
}

func (v *View) String() string {
	return fmt.Sprintf("(Sequence=%d, Round=%d)", v.Sequence, v.Round)
}
