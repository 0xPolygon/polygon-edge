package proto

import (
	"github.com/0xPolygon/minimal/types"
	"google.golang.org/protobuf/proto"
)

// PayloadNoSig returns the byte representation of the message request, without the signature field
func (m *MessageReq) PayloadNoSig() ([]byte, error) {
	m = m.Copy()
	m.Signature = ""

	data, err := proto.Marshal(m)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// FromAddr returns the from address in the message request
func (m *MessageReq) FromAddr() types.Address {
	return types.StringToAddress(m.From)
}

// ViewMsg generates a view object based on the passed in sequence and round
func ViewMsg(sequence, round uint64) *View {
	return &View{
		Sequence: sequence,
		Round:    round,
	}
}

// Copy makes a copy of the message request, and returns it
func (m *MessageReq) Copy() *MessageReq {
	return proto.Clone(m).(*MessageReq)
}

// Copy makes a copy of the candidate and returns it
func (c *Candidate) Copy() *Candidate {
	return proto.Clone(c).(*Candidate)
}

// Copy makes a copy of the view and returns it
func (v *View) Copy() *View {
	return proto.Clone(v).(*View)
}
