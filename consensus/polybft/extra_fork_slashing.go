package polybft

import (
	"github.com/0xPolygon/polygon-edge/consensus/polybft/slashing"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type ExtraHandlerSlashing struct {
}

func (e *ExtraHandlerSlashing) MarshalRLPWith(extra *Extra, ar *fastrlp.Arena) *fastrlp.Value {
	vv := extraMarshalBaseFields(extra, ar, ar.NewArray())

	if len(extra.DoubleSignEvidences) == 0 {
		vv.Set(ar.NewNullArray())
	} else {
		vv.Set(extra.DoubleSignEvidences.MarshalRLPWith(ar))
	}

	return vv
}

func (e *ExtraHandlerSlashing) UnmarshalRLPWith(extra *Extra, v *fastrlp.Value) error {
	elems, err := extraUnmarshalBaseFields(extra, v, 5)
	if err != nil {
		return err
	}

	if elems[4].Elems() > 0 {
		extra.DoubleSignEvidences = slashing.DoubleSignEvidences{}
		if err := extra.DoubleSignEvidences.UnmarshalRLPWith(elems[4]); err != nil {
			return err
		}
	}

	return nil
}

func (e *ExtraHandlerSlashing) ValidateAdditional(extra *Extra, header *types.Header) error {
	return nil
}

func (e *ExtraHandlerSlashing) GetIbftExtraClean(extra *Extra) *Extra {
	return &Extra{
		BlockNumber:         extra.BlockNumber,
		Parent:              extra.Parent,
		Validators:          extra.Validators,
		Checkpoint:          extra.Checkpoint,
		DoubleSignEvidences: extra.DoubleSignEvidences,
		Committed:           &Signature{},
	}
}
