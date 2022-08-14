package validators

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

// BLSValidator is a validator using ECDSA signing algorithm
type ECDSAValidator struct {
	Address types.Address
}

// NewECDSAValidator is a constructor of ECDSAValidator
func NewECDSAValidator(addr types.Address) *ECDSAValidator {
	return &ECDSAValidator{
		Address: addr,
	}
}

// Type returns the ValidatorType of ECDSAValidator
func (v *ECDSAValidator) Type() ValidatorType {
	return ECDSAValidatorType
}

// String returns string representation of ECDSAValidator
func (v *ECDSAValidator) String() string {
	return v.Address.String()
}

// Addr returns the validator address
func (v *ECDSAValidator) Addr() types.Address {
	return v.Address
}

// Copy returns copy of ECDSAValidator
func (v *ECDSAValidator) Copy() Validator {
	return &ECDSAValidator{
		Address: v.Address,
	}
}

// Equal checks the given validator matches with its data
func (v *ECDSAValidator) Equal(vr Validator) bool {
	vv, ok := vr.(*ECDSAValidator)
	if !ok {
		return false
	}

	return v.Address == vv.Address
}

// MarshalRLPWith is a RLP Marshaller
func (v *ECDSAValidator) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	vv.Set(arena.NewBytes(v.Address[:]))

	return vv
}

// UnmarshalRLPFrom is a RLP Unmarshaller
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

// Bytes returns bytes of ECDSAValidator in RLP encode
func (v *ECDSAValidator) Bytes() []byte {
	return types.MarshalRLPTo(v.MarshalRLPWith, nil)
}

// SetFromBytes parses given bytes in RLP encode and map to its fields
func (v *ECDSAValidator) SetFromBytes(input []byte) error {
	return types.UnmarshalRlp(v.UnmarshalRLPFrom, input)
}

// ECDSAValidators is a collection of ECDSAValidator
type ECDSAValidators []*ECDSAValidator

// Type returns the type of validator
func (vs *ECDSAValidators) Type() ValidatorType {
	return ECDSAValidatorType
}

// Len returns the size of its collection
func (vs *ECDSAValidators) Len() int {
	return len(*vs)
}

// Equal checks the given validators matches with its data
func (vs *ECDSAValidators) Equal(ts Validators) bool {
	vts, ok := ts.(*ECDSAValidators)
	if !ok {
		return false
	}

	if vs.Len() != vts.Len() {
		return false
	}

	for idx, vsv := range *vs {
		if tsv := (*vts)[idx]; !vsv.Equal(tsv) {
			return false
		}
	}

	return true
}

// Copy returns a copy of BLSValidators
func (vs *ECDSAValidators) Copy() Validators {
	clone := make(ECDSAValidators, vs.Len())

	for idx, val := range *vs {
		clone[idx], _ = val.Copy().(*ECDSAValidator)
	}

	return &clone
}

// At returns a validator at specified index in the collection
func (vs *ECDSAValidators) At(index uint64) Validator {
	return (*vs)[index]
}

// Index returns the index of the validator whose address
// matches with the given address
func (vs *ECDSAValidators) Index(addr types.Address) int64 {
	for i, v := range *vs {
		if v.Address == addr {
			return int64(i)
		}
	}

	return -1
}

// Includes return the bool indicating whether the validator
// whose address matches with the given address exists or not
func (vs *ECDSAValidators) Includes(addr types.Address) bool {
	return vs.Index(addr) != -1
}

// Add adds a validator into the collection
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

// Del removes a validator from the collection
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

// Merge introduces the given collection into its collection
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

// MarshalRLPWith is a RLP Marshaller
func (vs *ECDSAValidators) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	for _, v := range *vs {
		vv.Set(v.MarshalRLPWith(arena))
	}

	return vv
}

// UnmarshalRLPFrom is a RLP Unmarshaller
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
