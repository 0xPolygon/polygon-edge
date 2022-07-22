package validators

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type BLSValidator struct {
	Address      types.Address
	BLSPublicKey []byte
}

func ParseBLSValidator(s string) (*BLSValidator, error) {
	subValues := strings.Split(s, ":")

	if len(subValues) != 2 {
		return nil, fmt.Errorf("invalid validator format, expected [Validator Address]:[BLS Public Key]")
	}

	addrBytes, err := hex.DecodeString(strings.TrimPrefix(subValues[0], "0x"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse address: %w", err)
	}

	pubKeyBytes, err := hex.DecodeString(strings.TrimPrefix(subValues[1], "0x"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse BLS Public Key: %w", err)
	}

	return &BLSValidator{
		Address:      types.BytesToAddress(addrBytes),
		BLSPublicKey: pubKeyBytes,
	}, nil
}

func (v *BLSValidator) Addr() types.Address {
	return v.Address
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
		panic(fmt.Errorf("incorrect number of elements to decode BLSValidator, expected 2 but found %d", len(elems)))
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

type BLSValidatorSet []*BLSValidator

func (vs *BLSValidatorSet) Type() ValidatorType {
	return BLSValidatorType
}

func (vs *BLSValidatorSet) Len() int {
	return len(*vs)
}

func (vs *BLSValidatorSet) Equal(ts ValidatorSet) bool {
	vts, ok := ts.(*BLSValidatorSet)
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

func (vs *BLSValidatorSet) Copy() ValidatorSet {
	clone := make(BLSValidatorSet, vs.Len())

	for idx, val := range *vs {
		copy := *val

		clone[idx] = &copy
	}

	return &clone
}

func (vs *BLSValidatorSet) At(index uint64) Validator {
	return (*vs)[index]
}

func (vs *BLSValidatorSet) Index(addr types.Address) int64 {
	for i, v := range *vs {
		if v.Address == addr {
			return int64(i)
		}
	}

	return -1
}

func (vs *BLSValidatorSet) Includes(addr types.Address) bool {
	return vs.Index(addr) != -1
}

func (vs *BLSValidatorSet) Add(v Validator) error {
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

func (vs *BLSValidatorSet) Del(v Validator) error {
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

func (vs *BLSValidatorSet) Merge(vts ValidatorSet) error {
	targetSet, ok := vts.(*BLSValidatorSet)
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

func (vs *BLSValidatorSet) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	for _, v := range *vs {
		vv.Set(v.MarshalRLPWith(arena))
	}

	return vv
}

func (vs *BLSValidatorSet) UnmarshalRLPFrom(p *fastrlp.Parser, val *fastrlp.Value) error {
	elems, err := val.GetElems()
	if err != nil {
		return err
	}

	*vs = make(BLSValidatorSet, len(elems))

	for idx, e := range elems {
		val := &BLSValidator{}
		if err := val.UnmarshalRLPFrom(p, e); err != nil {
			return err
		}

		(*vs)[idx] = val
	}

	return nil
}
