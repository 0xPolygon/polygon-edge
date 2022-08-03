package validators

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type ECDSAValidator struct {
	Address types.Address
}

func NewECDSAValidator(addr types.Address) *ECDSAValidator {
	return &ECDSAValidator{
		Address: addr,
	}
}

func ParseECDSAValidator(s string) (*ECDSAValidator, error) {
	bytes, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		return nil, err
	}

	return &ECDSAValidator{
		Address: types.BytesToAddress(bytes),
	}, nil
}

func (v *ECDSAValidator) Type() ValidatorType {
	return ECDSAValidatorType
}

func (v *ECDSAValidator) String() string {
	return v.Address.String()
}

func (v *ECDSAValidator) Addr() types.Address {
	return v.Address
}

func (v *ECDSAValidator) Copy() Validator {
	return &ECDSAValidator{
		Address: v.Address,
	}
}

func (v *ECDSAValidator) Equal(vr Validator) bool {
	vv, ok := vr.(*ECDSAValidator)
	if !ok {
		return false
	}

	return v.Address == vv.Address
}

func (v *ECDSAValidator) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	vv.Set(arena.NewBytes(v.Address[:]))

	return vv
}

func (v *ECDSAValidator) UnmarshalRLPFrom(p *fastrlp.Parser, val *fastrlp.Value) error {
	elems, err := val.GetElems()
	if err != nil {
		return err
	}

	if len(elems) < 1 {
		return fmt.Errorf("incorrect number of elements to decode ECDSAValidator, expected 1 but found %d", len(elems))
	}

	if err := elems[0].GetAddr(v.Address[:]); err != nil {
		return fmt.Errorf("failed to decode Address: %w", err)
	}

	return nil
}

func (v *ECDSAValidator) Bytes() []byte {
	return types.MarshalRLPTo(v.MarshalRLPWith, nil)
}

func (v *ECDSAValidator) SetFromBytes(input []byte) error {
	return types.UnmarshalRlp(v.UnmarshalRLPFrom, input)
}

type ECDSAValidators []*ECDSAValidator

func (vs *ECDSAValidators) Type() ValidatorType {
	return ECDSAValidatorType
}

func (vs *ECDSAValidators) Len() int {
	return len(*vs)
}

func (vs *ECDSAValidators) Equal(ts Validators) bool {
	vts, ok := ts.(*ECDSAValidators)
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

func (vs *ECDSAValidators) Copy() Validators {
	clone := make(ECDSAValidators, vs.Len())

	for idx, val := range *vs {
		clone[idx], _ = val.Copy().(*ECDSAValidator)
	}

	return &clone
}

func (vs *ECDSAValidators) At(index uint64) Validator {
	return (*vs)[index]
}

func (vs *ECDSAValidators) Index(addr types.Address) int64 {
	for i, v := range *vs {
		if v.Address == addr {
			return int64(i)
		}
	}

	return -1
}

func (vs *ECDSAValidators) Includes(addr types.Address) bool {
	return vs.Index(addr) != -1
}

func (vs *ECDSAValidators) Add(v Validator) error {
	validator, ok := v.(*ECDSAValidator)
	if !ok {
		return ErrMismatchValidatorType
	}

	if vs.Includes(validator.Address) {
		return ErrValidatorAlreadyExists
	}

	(*vs) = append((*vs), validator)

	return nil
}

func (vs *ECDSAValidators) Del(v Validator) error {
	validator, ok := v.(*ECDSAValidator)
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

func (vs *ECDSAValidators) Merge(vts Validators) error {
	targetSet, ok := vts.(*ECDSAValidators)
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

func (vs *ECDSAValidators) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	for _, v := range *vs {
		vv.Set(v.MarshalRLPWith(arena))
	}

	return vv
}

func (vs *ECDSAValidators) UnmarshalRLPFrom(p *fastrlp.Parser, val *fastrlp.Value) error {
	elems, err := val.GetElems()
	if err != nil {
		return err
	}

	*vs = make(ECDSAValidators, len(elems))

	for idx, e := range elems {
		val := &ECDSAValidator{}
		if err := val.UnmarshalRLPFrom(p, e); err != nil {
			return err
		}

		(*vs)[idx] = val
	}

	return nil
}
