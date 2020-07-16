package core

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/0xPolygon/minimal/types"
)

// notice: the normal case have been tested in integration tests.
func TestHandleMsg(t *testing.T) {
	N := uint64(4)
	F := uint64(1)
	sys := NewTestSystemWithBackend(N, F)

	closer := sys.Run(true)
	defer closer()

	v0 := sys.backends[0]
	r0 := v0.engine.(*core)

	m, _ := Encode(&ibft.Subject{
		View: &ibft.View{
			Sequence: big.NewInt(0),
			Round:    big.NewInt(0),
		},
		Digest: types.StringToHash("1234567890"),
	})
	// with a matched payload. msgPreprepare should match with *istanbul.Preprepare in normal case.
	msg := &message{
		Code:          msgPreprepare,
		Msg:           m,
		Address:       v0.Address(),
		Signature:     []byte{},
		CommittedSeal: []byte{},
	}

	_, val := v0.Validators(nil).GetByAddress(v0.Address())
	if err := r0.handleCheckedMsg(msg, val); err != errFailedDecodePreprepare {
		t.Errorf("error mismatch: have %v, want %v", err, errFailedDecodePreprepare)
	}

	m, _ = Encode(&ibft.Preprepare{
		View: &ibft.View{
			Sequence: big.NewInt(0),
			Round:    big.NewInt(0),
		},
		Proposal: makeBlock(1),
	})
	// with a unmatched payload. msgPrepare should match with *istanbul.Subject in normal case.
	msg = &message{
		Code:          msgPrepare,
		Msg:           m,
		Address:       v0.Address(),
		Signature:     []byte{},
		CommittedSeal: []byte{},
	}

	_, val = v0.Validators(nil).GetByAddress(v0.Address())
	if err := r0.handleCheckedMsg(msg, val); err != errFailedDecodePrepare {
		t.Errorf("error mismatch: have %v, want %v", err, errFailedDecodePreprepare)
	}

	m, _ = Encode(&ibft.Preprepare{
		View: &ibft.View{
			Sequence: big.NewInt(0),
			Round:    big.NewInt(0),
		},
		Proposal: makeBlock(2),
	})
	// with a unmatched payload. istanbul.MsgCommit should match with *istanbul.Subject in normal case.
	msg = &message{
		Code:          msgCommit,
		Msg:           m,
		Address:       v0.Address(),
		Signature:     []byte{},
		CommittedSeal: []byte{},
	}

	_, val = v0.Validators(nil).GetByAddress(v0.Address())
	if err := r0.handleCheckedMsg(msg, val); err != errFailedDecodeCommit {
		t.Errorf("error mismatch: have %v, want %v", err, errFailedDecodeCommit)
	}

	m, _ = Encode(&ibft.Preprepare{
		View: &ibft.View{
			Sequence: big.NewInt(0),
			Round:    big.NewInt(0),
		},
		Proposal: makeBlock(3),
	})
	// invalid message code. message code is not exists in list
	msg = &message{
		Code:          uint64(99),
		Msg:           m,
		Address:       v0.Address(),
		Signature:     []byte{},
		CommittedSeal: []byte{},
	}

	_, val = v0.Validators(nil).GetByAddress(v0.Address())
	if err := r0.handleCheckedMsg(msg, val); err == nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	// with malicious payload
	if err := r0.handleMsg([]byte{1}); err == nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
}
