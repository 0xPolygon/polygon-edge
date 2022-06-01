package signer

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/validators"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

var (
	// IstanbulDigest represents a hash of "Istanbul practical byzantine fault tolerance"
	// to identify whether the block is from Istanbul consensus engine
	IstanbulDigest = types.StringToHash("0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365")

	// IstanbulExtraVanity represents a fixed number of extra-data bytes reserved for proposer vanity
	IstanbulExtraVanity = 32

	// IstanbulExtraSeal represents the fixed number of extra-data bytes reserved for proposer seal
	IstanbulExtraSeal = 65

	zeroBytes = make([]byte, 32)
)

// IstanbulExtra defines the structure of the extra field for Istanbul
type IstanbulExtra struct {
	Validators          validators.ValidatorSet
	Seal                []byte
	CommittedSeal       Sealer
	ParentCommittedSeal Sealer
}

type Sealer interface {
	MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value
	UnmarshalRLPFrom(*fastrlp.Parser, *fastrlp.Value) error
}

// MarshalRLPTo defines the marshal function wrapper for IstanbulExtra
func (i *IstanbulExtra) MarshalRLPTo(dst []byte) []byte {
	return types.MarshalRLPTo(i.MarshalRLPWith, dst)
}

// MarshalRLPWith defines the marshal function implementation for IstanbulExtra
func (i *IstanbulExtra) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	vv := ar.NewArray()

	// Validators
	vv.Set(i.Validators.MarshalRLPWith(ar))

	// Seal
	if len(i.Seal) == 0 {
		vv.Set(ar.NewNull())
	} else {
		vv.Set(ar.NewBytes(i.Seal))
	}

	// CommittedSeal
	vv.Set(i.CommittedSeal.MarshalRLPWith(ar))

	// ParentCommittedSeal
	if i.ParentCommittedSeal != nil {
		vv.Set(i.ParentCommittedSeal.MarshalRLPWith(ar))
	}

	return vv
}

// UnmarshalRLP defines the unmarshal function wrapper for IstanbulExtra
func (i *IstanbulExtra) UnmarshalRLP(input []byte) error {
	return types.UnmarshalRlp(i.UnmarshalRLPFrom, input)
}

// UnmarshalRLPFrom defines the unmarshal implementation for IstanbulExtra
func (i *IstanbulExtra) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if num := len(elems); num != 3 && num != 4 {
		return fmt.Errorf("not enough elements to decode istambul extra, expected 3 or 4 but found %d", num)
	}

	// Validators
	if err := i.Validators.UnmarshalRLPFrom(p, elems[0]); err != nil {
		return err
	}

	// Seal
	{
		if i.Seal, err = elems[1].GetBytes(i.Seal); err != nil {
			return fmt.Errorf("failed to decode Seal: %w", err)
		}
	}

	// Committed
	if err := i.CommittedSeal.UnmarshalRLPFrom(p, elems[2]); err != nil {
		return err
	}

	// LastCommitted
	if len(elems) >= 4 {
		if err := i.ParentCommittedSeal.UnmarshalRLPFrom(p, elems[3]); err != nil {
			return err
		}
	}

	return nil
}

// putIbftExtra sets the IBFT extra data field into the header
func putIbftExtra(h *types.Header, istanbulExtra *IstanbulExtra) {
	// Pad zeros to the right up to istanbul vanity
	extra := h.ExtraData
	if len(extra) < IstanbulExtraVanity {
		extra = append(extra, zeroBytes[:IstanbulExtraVanity-len(extra)]...)
	} else {
		extra = extra[:IstanbulExtraVanity]
	}

	h.ExtraData = istanbulExtra.MarshalRLPTo(extra)
}

// TODO: fix this
// GetIbftExtra extracts the istanbul extra data from the given header
func GetIbftExtra(h *types.Header) (*IstanbulExtra, error) {
	if len(h.ExtraData) < IstanbulExtraVanity {
		return nil, fmt.Errorf(
			"wrong extra size, expected greater than or equal to %d but actual %d",
			IstanbulExtraVanity,
			len(h.ExtraData),
		)
	}

	data := h.ExtraData[IstanbulExtraVanity:]
	extra := &IstanbulExtra{
		// TODO
		CommittedSeal:       &BLSSeal{},
		ParentCommittedSeal: &BLSSeal{},
	}

	if err := extra.UnmarshalRLP(data); err != nil {
		return nil, err
	}

	return extra, nil
}

// unpackSealFromIbftExtra extracts Seal from the istanbul extra of the given Header
func unpackSealFromIbftExtra(h *types.Header) ([]byte, error) {
	extra, err := GetIbftExtra(h)
	if err != nil {
		return nil, err
	}

	return extra.Seal, nil
}

func unpackValidatorsFromIbftExtra(h *types.Header) (validators.ValidatorSet, error) {
	extra, err := GetIbftExtra(h)
	if err != nil {
		return nil, err
	}

	return extra.Validators, nil
}

// unpackCommittedSealFromIbftExtra extracts CommittedSeal from the istanbul extra of the given Header
func unpackCommittedSealFromIbftExtra(h *types.Header) (Sealer, error) {
	extra, err := GetIbftExtra(h)
	if err != nil {
		return nil, err
	}

	return extra.CommittedSeal, nil
}

// unpackParentCommittedSealFromIbftExtra extracts ParentCommittedSeal from the istanbul extra of the given Header
func unpackParentCommittedSealFromIbftExtra(h *types.Header) (Sealer, error) {
	extra, err := GetIbftExtra(h)
	if err != nil {
		return nil, err
	}

	return extra.ParentCommittedSeal, nil
}

// packFieldIntoIbftExtra is a helper function to update fields in the istanbul Extra of the given header
func packFieldIntoIbftExtra(h *types.Header, updateFn func(*IstanbulExtra)) error {
	extra, err := GetIbftExtra(h)
	if err != nil {
		return err
	}

	updateFn(extra)

	putIbftExtra(h, extra)

	return nil
}

// packSealIntoIbftExtra sets the seal to Seal field in istanbul extra of the given header
func packSealIntoIbftExtra(h *types.Header, seal []byte) error {
	return packFieldIntoIbftExtra(h, func(extra *IstanbulExtra) {
		extra.Seal = seal
	})
}

// packCommittedSealIntoIbftExtra sets the seals to CommittedSeal field in istanbul extra of the given header
func packCommittedSealIntoIbftExtra(h *types.Header, seals Sealer) error {
	return packFieldIntoIbftExtra(h, func(extra *IstanbulExtra) {
		extra.CommittedSeal = seals
	})
}
