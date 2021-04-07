package proto

import "github.com/0xPolygon/minimal/types"

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

func ViewMsg(sequence, round uint64) *View {
	return &View{
		Sequence: sequence,
		Round:    round,
	}
}
