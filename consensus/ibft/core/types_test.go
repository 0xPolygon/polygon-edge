package core

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/0xPolygon/minimal/types"
)

func testPreprepare(t *testing.T) {
	pp := &ibft.Preprepare{
		View: &ibft.View{
			Round:    big.NewInt(1),
			Sequence: big.NewInt(2),
		},
		Proposal: makeBlock(1),
	}
	prepreparePayload, _ := Encode(pp)

	m := &message{
		Code:    msgPreprepare,
		Msg:     prepreparePayload,
		Address: types.StringToAddress("0x1234567890"),
	}

	msgPayload, err := m.Payload()
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	decodedMsg := new(message)
	err = decodedMsg.FromPayload(msgPayload, nil)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	var decodedPP *ibft.Preprepare
	err = decodedMsg.Decode(&decodedPP)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	// if block is encoded/decoded by rlp, we cannot to compare interface data type using reflect.DeepEqual. (like istanbul.Proposal)
	// so individual comparison here.
	if !reflect.DeepEqual(pp.Proposal.Hash(), decodedPP.Proposal.Hash()) {
		t.Errorf("proposal hash mismatch: have %v, want %v", decodedPP.Proposal.Hash(), pp.Proposal.Hash())
	}

	if !reflect.DeepEqual(pp.View, decodedPP.View) {
		t.Errorf("view mismatch: have %v, want %v", decodedPP.View, pp.View)
	}

	if !reflect.DeepEqual(pp.Proposal.Number(), decodedPP.Proposal.Number()) {
		t.Errorf("proposal number mismatch: have %v, want %v", decodedPP.Proposal.Number(), pp.Proposal.Number())
	}
}

func testSubject(t *testing.T) {
	s := &ibft.Subject{
		View: &ibft.View{
			Round:    big.NewInt(1),
			Sequence: big.NewInt(2),
		},
		Digest: types.StringToHash("1234567890"),
	}

	subjectPayload, _ := Encode(s)

	m := &message{
		Code:    msgPreprepare,
		Msg:     subjectPayload,
		Address: types.StringToAddress("0x1234567890"),
	}

	msgPayload, err := m.Payload()
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	decodedMsg := new(message)
	err = decodedMsg.FromPayload(msgPayload, nil)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	var decodedSub *ibft.Subject
	err = decodedMsg.Decode(&decodedSub)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	if !reflect.DeepEqual(s, decodedSub) {
		t.Errorf("subject mismatch: have %v, want %v", decodedSub, s)
	}
}

func testSubjectWithSignature(t *testing.T) {
	s := &ibft.Subject{
		View: &ibft.View{
			Round:    big.NewInt(1),
			Sequence: big.NewInt(2),
		},
		Digest: types.StringToHash("1234567890"),
	}
	expectedSig := []byte{0x01}

	subjectPayload, _ := Encode(s)
	// 1. Encode test
	m := &message{
		Code:          msgPreprepare,
		Msg:           subjectPayload,
		Address:       types.StringToAddress("0x1234567890"),
		Signature:     expectedSig,
		CommittedSeal: []byte{},
	}

	msgPayload, err := m.Payload()
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	// 2. Decode test
	// 2.1 Test normal validate func
	decodedMsg := new(message)
	err = decodedMsg.FromPayload(msgPayload, func(data []byte, sig []byte) (types.Address, error) {
		return types.Address{}, nil
	})
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	if !reflect.DeepEqual(decodedMsg, m) {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	// 2.2 Test nil validate func
	decodedMsg = new(message)
	err = decodedMsg.FromPayload(msgPayload, nil)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(decodedMsg, m) {
		t.Errorf("message mismatch: have %v, want %v", decodedMsg, m)
	}

	// 2.3 Test failed validate func
	decodedMsg = new(message)
	err = decodedMsg.FromPayload(msgPayload, func(data []byte, sig []byte) (types.Address, error) {
		return types.Address{}, ibft.ErrUnauthorizedAddress
	})
	if err != ibft.ErrUnauthorizedAddress {
		t.Errorf("error mismatch: have %v, want %v", err, ibft.ErrUnauthorizedAddress)
	}
}

func TestMessageEncodeDecode(t *testing.T) {
	testPreprepare(t)
	testSubject(t)
	testSubjectWithSignature(t)
}
