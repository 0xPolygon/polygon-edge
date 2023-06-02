package polybft

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/forkmanager"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

func init() {
	// for tests
	forkManagerInstance.RegisterFork(forkmanager.InitialFork)
	_ = forkManagerInstance.RegisterHandler(forkmanager.InitialFork, extraHandler, &ExtraHandlerBase{})
	_ = forkManagerInstance.ActivateFork(forkmanager.InitialFork, 0)
}

const extraHandler forkmanager.HandlerDesc = "extra"

type IExtraHandler interface {
	MarshalRLPWith(extra *Extra, ar *fastrlp.Arena) *fastrlp.Value
	UnmarshalRLPWith(extra *Extra, v *fastrlp.Value) error
	ValidateAdditional(extra *Extra, header *types.Header) error
	GetIbftExtraClean(extra *Extra) *Extra
}

var forkManagerInstance = forkmanager.GetInstance()

func GetExtraHandler(blockNumber uint64) IExtraHandler {
	handler := forkManagerInstance.GetHandler(extraHandler, blockNumber)

	return handler.(IExtraHandler) //nolint:forcetypeassert
}

type ExtraHandlerBase struct {
}

// MarshalRLPWith defines the marshal function implementation for Extra
func (e *ExtraHandlerBase) MarshalRLPWith(extra *Extra, ar *fastrlp.Arena) *fastrlp.Value {
	return extraMarshalBaseFields(extra, ar, ar.NewArray())
}

func (e *ExtraHandlerBase) UnmarshalRLPWith(extra *Extra, v *fastrlp.Value) error {
	_, err := extraUnmarshalBaseFields(extra, v, 4)

	return err
}

func (e *ExtraHandlerBase) ValidateAdditional(extra *Extra, header *types.Header) error {
	return nil
}

func (e *ExtraHandlerBase) GetIbftExtraClean(extra *Extra) *Extra {
	return &Extra{
		BlockNumber: extra.BlockNumber,
		Parent:      extra.Parent,
		Validators:  extra.Validators,
		Checkpoint:  extra.Checkpoint,
		Committed:   &Signature{},
	}
}

func extraMarshalBaseFields(extra *Extra, ar *fastrlp.Arena, vv *fastrlp.Value) *fastrlp.Value {
	if extra.Validators == nil {
		vv.Set(ar.NewNullArray())
	} else {
		vv.Set(extra.Validators.MarshalRLPWith(ar))
	}

	if extra.Parent == nil {
		vv.Set(ar.NewNullArray())
	} else {
		vv.Set(extra.Parent.MarshalRLPWith(ar))
	}

	if extra.Committed == nil {
		vv.Set(ar.NewNullArray())
	} else {
		vv.Set(extra.Committed.MarshalRLPWith(ar))
	}

	if extra.Checkpoint == nil {
		vv.Set(ar.NewNullArray())
	} else {
		vv.Set(extra.Checkpoint.MarshalRLPWith(ar))
	}

	return vv
}

func extraUnmarshalBaseFields(extra *Extra, v *fastrlp.Value, expectedCnt int) ([]*fastrlp.Value, error) {
	elems, err := v.GetElems()
	if err != nil {
		return nil, err
	}

	if num := len(elems); num != expectedCnt {
		return nil, fmt.Errorf("incorrect elements count to decode extra, expected %d but found %d", expectedCnt, num)
	}

	if elems[0].Elems() > 0 {
		extra.Validators = &validator.ValidatorSetDelta{}
		if err := extra.Validators.UnmarshalRLPWith(elems[0]); err != nil {
			return nil, err
		}
	}

	if elems[1].Elems() > 0 {
		extra.Parent = &Signature{}
		if err := extra.Parent.UnmarshalRLPWith(elems[1]); err != nil {
			return nil, err
		}
	}

	if elems[2].Elems() > 0 {
		extra.Committed = &Signature{}
		if err := extra.Committed.UnmarshalRLPWith(elems[2]); err != nil {
			return nil, err
		}
	}

	if elems[3].Elems() > 0 {
		extra.Checkpoint = &CheckpointData{}
		if err := extra.Checkpoint.UnmarshalRLPWith(elems[3]); err != nil {
			return nil, err
		}
	}

	return elems, nil
}
