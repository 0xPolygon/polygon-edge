package core

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/0xPolygon/minimal/types"
	"github.com/ethereum/go-ethereum/rlp"
)

func TestMessageSetWithPreprepare(t *testing.T) {
	valSet := newTestValidatorSet(4)

	ms := newMessageSet(valSet)

	view := &ibft.View{
		Round:    new(big.Int),
		Sequence: new(big.Int),
	}
	pp := &ibft.Preprepare{
		View:     view,
		Proposal: makeBlock(1),
	}

	rawPP, err := rlp.EncodeToBytes(pp)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	msg := &message{
		Code:    msgPreprepare,
		Msg:     rawPP,
		Address: valSet.GetProposer().Address(),
	}

	err = ms.Add(msg)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	err = ms.Add(msg)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	if ms.Size() != 1 {
		t.Errorf("the size of message set mismatch: have %v, want 1", ms.Size())
	}
}

func TestMessageSetWithSubject(t *testing.T) {
	valSet := newTestValidatorSet(4)

	ms := newMessageSet(valSet)

	view := &ibft.View{
		Round:    new(big.Int),
		Sequence: new(big.Int),
	}

	sub := &ibft.Subject{
		View:   view,
		Digest: types.StringToHash("1234567890"),
	}

	rawSub, err := rlp.EncodeToBytes(sub)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	msg := &message{
		Code:    msgPrepare,
		Msg:     rawSub,
		Address: valSet.GetProposer().Address(),
	}

	err = ms.Add(msg)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	err = ms.Add(msg)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	if ms.Size() != 1 {
		t.Errorf("the size of message set mismatch: have %v, want 1", ms.Size())
	}
}
