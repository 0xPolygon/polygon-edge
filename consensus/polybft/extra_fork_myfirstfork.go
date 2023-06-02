package polybft

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type ExtraHandlerMyFirstFork struct {
}

// MarshalRLPWith defines the marshal function implementation for Extra
func (e *ExtraHandlerMyFirstFork) MarshalRLPWith(extra *Extra, ar *fastrlp.Arena) *fastrlp.Value {
	vv := extraMarshalBaseFields(extra, ar, ar.NewArray())

	vv.Set(ar.NewString(extra.Dummy1))

	return vv
}

// UnmarshalRLPWith defines the unmarshal implementation for Extra
func (e *ExtraHandlerMyFirstFork) UnmarshalRLPWith(extra *Extra, v *fastrlp.Value) error {
	elems, err := extraUnmarshalBaseFields(extra, v, 5)
	if err != nil {
		return err
	}

	extra.Dummy1, err = elems[4].GetString()
	if err != nil {
		return fmt.Errorf("dummy1 bytes: %w", err)
	}

	return nil
}

func (e *ExtraHandlerMyFirstFork) ValidateAdditional(extra *Extra, header *types.Header) error {
	if extra.Dummy1 == "" {
		return fmt.Errorf("dummy1 is empty for block %d", header.Number)
	}

	return nil
}

func (e *ExtraHandlerMyFirstFork) GetIbftExtraClean(extra *Extra) *Extra {
	return &Extra{
		BlockNumber: extra.BlockNumber,
		Parent:      extra.Parent,
		Validators:  extra.Validators,
		Checkpoint:  extra.Checkpoint,
		Committed:   &Signature{},
		Dummy1:      extra.Dummy1,
	}
}
