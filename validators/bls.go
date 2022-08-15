package validators

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

// BLSValidator is a validator using BLS signing algorithm
type BLSValidator struct {
	Address      types.Address
	BLSPublicKey []byte
}

// NewBLSValidator is a constructor of BLSValidator
func NewBLSValidator(addr types.Address, blsPubkey []byte) *BLSValidator {
	return &BLSValidator{
		Address:      addr,
		BLSPublicKey: blsPubkey,
	}
}

// Type returns the ValidatorType of BLSValidator
func (v *BLSValidator) Type() ValidatorType {
	return BLSValidatorType
}

// String returns string representation of BLSValidator
// Format => [Address]:[BLSPublicKey]
func (v *BLSValidator) String() string {
	return fmt.Sprintf(
		"%s:0x%s",
		v.Address.String(),
		hex.EncodeToString(v.BLSPublicKey),
	)
}

// Addr returns the validator address
func (v *BLSValidator) Addr() types.Address {
	return v.Address
}

// Copy returns copy of BLS Validator
func (v *BLSValidator) Copy() Validator {
	pubkey := make([]byte, len(v.BLSPublicKey))
	copy(pubkey, v.BLSPublicKey)

	return &BLSValidator{
		Address:      v.Address,
		BLSPublicKey: pubkey,
	}
}

// Equal checks the given validator matches with its data
func (v *BLSValidator) Equal(vr Validator) bool {
	vv, ok := vr.(*BLSValidator)
	if !ok {
		return false
	}

	return v.Address == vv.Address && bytes.Equal(v.BLSPublicKey, vv.BLSPublicKey)
}

// MarshalRLPWith is a RLP Marshaller
func (v *BLSValidator) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	vv.Set(arena.NewBytes(v.Address[:]))
	vv.Set(arena.NewBytes(v.BLSPublicKey))

	return vv
}

// UnmarshalRLPFrom is a RLP Unmarshaller
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

// Bytes returns bytes of BLSValidator in RLP encode
func (v *BLSValidator) Bytes() []byte {
	return types.MarshalRLPTo(v.MarshalRLPWith, nil)
}

// SetFromBytes parses given bytes in RLP encode and map to its fields
func (v *BLSValidator) SetFromBytes(input []byte) error {
	return types.UnmarshalRlp(v.UnmarshalRLPFrom, input)
}

// BLSValidators is a collection of BLSValidator
type BLSValidators []*BLSValidator

// Type returns the type of validator
func (vs *BLSValidators) Type() ValidatorType {
	return BLSValidatorType
}

// Len returns the size of its collection
func (vs *BLSValidators) Len() int {
	return len(*vs)
}

// Equal checks the given validators matches with its data
func (vs *BLSValidators) Equal(ts Validators) bool {
	vts, ok := ts.(*BLSValidators)
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
func (vs *BLSValidators) Copy() Validators {
	clone := make(BLSValidators, vs.Len())

	for idx, val := range *vs {
		clone[idx], _ = val.Copy().(*BLSValidator)
	}

	return &clone
}

// At returns a validator at specified index in the collection
func (vs *BLSValidators) At(index uint64) Validator {
	return (*vs)[index]
}

// Index returns the index of the validator whose address matches with the given address
func (vs *BLSValidators) Index(addr types.Address) int64 {
	for i, v := range *vs {
		if v.Address == addr {
			return int64(i)
		}
	}

	return -1
}

// Includes return the bool indicating whether the validator
// whose address matches with the given address exists or not
func (vs *BLSValidators) Includes(addr types.Address) bool {
	return vs.Index(addr) != -1
}

// Add adds a validator into the collection
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

// Del removes a validator from the collection
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

// Merge introduces the given collection into its collection
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

// MarshalRLPWith is a RLP Marshaller
func (vs *BLSValidators) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	for _, v := range *vs {
		vv.Set(v.MarshalRLPWith(arena))
	}

	return vv
}

// UnmarshalRLPFrom is a RLP Unmarshaller
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
