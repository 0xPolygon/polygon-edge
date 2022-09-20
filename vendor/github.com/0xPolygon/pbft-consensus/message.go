package pbft

import (
	"bytes"
	"fmt"
)

type MsgType int32

const (
	MessageReq_RoundChange MsgType = 0
	MessageReq_Preprepare  MsgType = 1
	MessageReq_Commit      MsgType = 2
	MessageReq_Prepare     MsgType = 3
)

func (m MsgType) String() string {
	switch m {
	case MessageReq_RoundChange:
		return "RoundChange"
	case MessageReq_Preprepare:
		return "Preprepare"
	case MessageReq_Commit:
		return "Commit"
	case MessageReq_Prepare:
		return "Prepare"
	default:
		panic(fmt.Sprintf("BUG: Bad msgtype %d", m))
	}
}

type MessageReq struct {
	// type is the type of the message
	Type MsgType `json:"type"`

	// from is the address of the sender
	From NodeID `json:"from"`

	// seal is the committed seal for the proposal (only for commit messages)
	Seal []byte `json:"seal"`

	// view is the view assigned to the message
	View *View `json:"view"`

	// hash of the proposal
	Hash []byte `json:"hash"`

	// proposal is the arbitrary data proposal (only for preprepare messages)
	Proposal []byte `json:"proposal"`
}

func (m MessageReq) String() string {
	return fmt.Sprintf("message - type: %s from: %s, view: %v, proposal: %v, hash: %v, seal: %v", m.Type, m.From, m.View, m.Proposal, m.Hash, m.Seal)
}

func (m *MessageReq) Validate() error {
	// Hash field has to exist for state != RoundStateChange
	if m.Type != MessageReq_RoundChange {
		if m.Hash == nil {
			return fmt.Errorf("hash is empty for type %s", m.Type.String())
		}
	}

	// TODO
	return nil
}

func (m *MessageReq) SetProposal(proposal []byte) {
	m.Proposal = append([]byte{}, proposal...)
}

func (m *MessageReq) Copy() *MessageReq {
	mm := new(MessageReq)
	*mm = *m
	if m.View != nil {
		mm.View = m.View.Copy()
	}

	if m.Proposal != nil {
		mm.SetProposal(m.Proposal)
	}

	if m.Seal != nil {
		mm.Seal = append([]byte{}, m.Seal...)
	}

	return mm
}

// Equal compares if two messages are equal
func (m *MessageReq) Equal(other *MessageReq) bool {
	return other != nil &&
		m.Type == other.Type && m.From == other.From &&
		bytes.Equal(m.Proposal, other.Proposal) &&
		bytes.Equal(m.Hash, other.Hash) &&
		bytes.Equal(m.Seal, other.Seal) &&
		m.View.Round == other.View.Round &&
		m.View.Sequence == other.View.Sequence
}
