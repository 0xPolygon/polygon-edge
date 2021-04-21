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
