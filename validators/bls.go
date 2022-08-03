package validators

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type BLSValidator struct {
	Address      types.Address
	BLSPublicKey []byte
}

func (v *BLSValidator) String() string {
	return fmt.Sprintf(
		"%s:%s",
		v.Address.String(),
		hex.EncodeToString(v.BLSPublicKey),
	)
}

func (v *BLSValidator) Type() ValidatorType {
	return BLSValidatorType
}

func (v *BLSValidator) Addr() types.Address {
	return v.Address
}

func (v *BLSValidator) Copy() Validator {
	pubkey := make([]byte, len(v.BLSPublicKey))
	copy(pubkey, v.BLSPublicKey)

	return &BLSValidator{
		Address:      v.Address,
		BLSPublicKey: pubkey,
	}
}

func (v *BLSValidator) Equal(vr Validator) bool {
	vv, ok := vr.(*BLSValidator)
	if !ok {
		return false
	}

	return v.Address == vv.Address && bytes.Equal(v.BLSPublicKey, vv.BLSPublicKey)
}

func (v *BLSValidator) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	vv.Set(arena.NewBytes(v.Address[:]))
	vv.Set(arena.NewBytes(v.BLSPublicKey))

	return vv
}

func (v *BLSValidator) UnmarshalRLPFrom(p *fastrlp.Parser, val *fastrlp.Value) error {
	elems, err := val.GetElems()
	if err != nil {
		return err
	}

	if len(elems) < 2 {
		return fmt.Errorf("incorrect number of elements to decode BLSValidator, expected 2 but found %d", len(elems))
	}

	if err := elems[0].GetAddr(v.Address[:]); err != nil {
		return fmt.Errorf("failed to decode Address: %w", err)
	}

	if v.BLSPublicKey, err = elems[1].GetBytes(v.BLSPublicKey); err != nil {
		return fmt.Errorf("failed to decode BLSPublicKey: %w", err)
	}

	return nil
}

func (v *BLSValidator) Bytes() []byte {
	return types.MarshalRLPTo(v.MarshalRLPWith, nil)
}

func (v *BLSValidator) SetFromBytes(input []byte) error {
	return types.UnmarshalRlp(v.UnmarshalRLPFrom, input)
}

type BLSValidators []*BLSValidator

func (vs *BLSValidators) Type() ValidatorType {
	return BLSValidatorType
}

func (vs *BLSValidators) Len() int {
	return len(*vs)
}

func (vs *BLSValidators) Equal(ts Validators) bool {
	vts, ok := ts.(*BLSValidators)
	if !ok {
		return false
	}

	if vs.Len() != vts.Len() {
		return false
	}

	for idx, vsv := range *vs {
		tsv := (*vts)[idx]
		if !vsv.Equal(tsv) {
			return false
		}
	}

	return true
}

func (vs *BLSValidators) Copy() Validators {
	clone := make(BLSValidators, vs.Len())

	for idx, val := range *vs {
		clone[idx], _ = val.Copy().(*BLSValidator)
	}

	return &clone
}

func (vs *BLSValidators) At(index uint64) Validator {
	return (*vs)[index]
}

func (vs *BLSValidators) Index(addr types.Address) int64 {
	for i, v := range *vs {
		if v.Address == addr {
			return int64(i)
		}
	}

	return -1
}

func (vs *BLSValidators) Includes(addr types.Address) bool {
	return vs.Index(addr) != -1
}

func (vs *BLSValidators) Add(v Validator) error {
	validator, ok := v.(*BLSValidator)
	if !ok {
		return ErrMismatchValidatorType
	}

	if vs.Includes(validator.Address) {
		return ErrValidatorAlreadyExists
	}

	(*vs) = append((*vs), validator)

	return nil
}

func (vs *BLSValidators) Del(v Validator) error {
	validator, ok := v.(*BLSValidator)
	if !ok {
		return ErrMismatchValidatorType
	}

	index := vs.Index(validator.Address)

	if index == -1 {
		return ErrValidatorNotFound
	}

	(*vs) = append((*vs)[:index], (*vs)[index+1:]...)

	return nil
}

func (vs *BLSValidators) Merge(vts Validators) error {
	targetSet, ok := vts.(*BLSValidators)
	if !ok {
		return ErrMismatchValidatorSetType
	}

	for _, tsv := range *targetSet {
		if vs.Includes(tsv.Address) {
			continue
		}

		if err := vs.Add(tsv); err != nil {
			return err
		}
	}

	return nil
}

func (vs *BLSValidators) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	for _, v := range *vs {
		vv.Set(v.MarshalRLPWith(arena))
	}

	return vv
}

func (vs *BLSValidators) UnmarshalRLPFrom(p *fastrlp.Parser, val *fastrlp.Value) error {
	elems, err := val.GetElems()
	if err != nil {
		return err
	}

	*vs = make(BLSValidators, len(elems))

	for idx, e := range elems {
		val := &BLSValidator{}
		if err := val.UnmarshalRLPFrom(p, e); err != nil {
			return err
		}

		(*vs)[idx] = val
	}

	return nil
}
