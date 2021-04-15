package proto

import (
	"github.com/0xPolygon/minimal/types"
	"google.golang.org/protobuf/proto"
)

// PayloadNoSig returns the byte to sign
func (m *MessageReq) PayloadNoSig() ([]byte, error) {
	m = m.Copy()
	m.Signature = ""

	data, err := proto.Marshal(m)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (m *MessageReq) FromAddr() types.Address {
	return types.StringToAddress(m.From)
}

/*
func (m *MessageReq) View() *View {
	var view *View
	switch obj := m.Message.(type) {
	case *MessageReq_Preprepare:
		view = obj.Preprepare.View

	case *MessageReq_Prepare:
		view = obj.Prepare.View

	case *MessageReq_Commit:
		view = obj.Commit.View

	case *MessageReq_RoundChange:
		view = obj.RoundChange.View
	}
	return view
}
*/

/*
func PreprepareMsg(from types.Address, preprepare *Preprepare) *MessageReq {
	return &MessageReq{
		Message: &MessageReq_Preprepare{
			Preprepare: preprepare,
		},
		From: from.String(),
	}
}

func Commit(from types.Address, subject *Subject) *MessageReq {
	return &MessageReq{
		Message: &MessageReq_Commit{
			Commit: subject,
		},
		From: from.String(),
	}
}

func Prepare(from types.Address, subject *Subject) *MessageReq {
	return &MessageReq{
		Message: &MessageReq_Prepare{
			Prepare: subject,
		},
		From: from.String(),
	}
}

func RoundChange(from types.Address, subject *Subject) *MessageReq {
	return &MessageReq{
		Message: &MessageReq_RoundChange{
			RoundChange: subject,
		},
		From: from.String(),
	}
}
*/

func ViewMsg(sequence, round uint64) *View {
	return &View{
		Sequence: sequence,
		Round:    round,
	}
}

func (m *MessageReq) Copy() *MessageReq {
	return proto.Clone(m).(*MessageReq)
}

func (c *Candidate) Copy() *Candidate {
	return proto.Clone(c).(*Candidate)
}

func (v *View) Copy() *View {
	return proto.Clone(v).(*View)
}
