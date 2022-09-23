package validators

import (
	"encoding/json"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type Set struct {
	ValidatorType ValidatorType
	Validators    []Validator
}

// Type returns the type of validator
func (s *Set) Type() ValidatorType {
	return s.ValidatorType
}

// Len returns the size of its collection
func (s *Set) Len() int {
	return len(s.Validators)
}

// Equal checks the given validators matches with its data
func (s *Set) Equal(ss Validators) bool {
	if s.ValidatorType != ss.Type() {
		return false
	}

	if s.Len() != ss.Len() {
		return false
	}

	for idx := 0; idx < s.Len(); idx++ {
		val1 := s.At(uint64(idx))
		val2 := ss.At(uint64(idx))

		if !val1.Equal(val2) {
			return false
		}
	}

	return true
}

// Copy returns a copy of BLSValidators
func (s *Set) Copy() Validators {
	cloneValidators := make([]Validator, len(s.Validators))

	for idx, val := range s.Validators {
		cloneValidators[idx] = val.Copy()
	}

	return &Set{
		ValidatorType: s.ValidatorType,
		Validators:    cloneValidators,
	}
}

// At returns a validator at specified index in the collection
func (s *Set) At(index uint64) Validator {
	return s.Validators[index]
}

// Index returns the index of the validator whose address matches with the given address
func (s *Set) Index(addr types.Address) int64 {
	for i, val := range s.Validators {
		if val.Addr() == addr {
			return int64(i)
		}
	}

	return -1
}

// Includes return the bool indicating whether the validator
// whose address matches with the given address exists or not
func (s *Set) Includes(addr types.Address) bool {
	return s.Index(addr) != -1
}

// Add adds a validator into the collection
func (s *Set) Add(val Validator) error {
	if s.ValidatorType != val.Type() {
		return ErrMismatchValidatorType
	}

	if s.Includes(val.Addr()) {
		return ErrValidatorAlreadyExists
	}

	s.Validators = append(s.Validators, val)

	return nil
}

// Del removes a validator from the collection
func (s *Set) Del(val Validator) error {
	if s.ValidatorType != val.Type() {
		return ErrMismatchValidatorType
	}

	index := s.Index(val.Addr())

	if index == -1 {
		return ErrValidatorNotFound
	}

	s.Validators = append(s.Validators[:index], s.Validators[index+1:]...)

	return nil
}

// Merge introduces the given collection into its collection
func (s *Set) Merge(ss Validators) error {
	if s.ValidatorType != ss.Type() {
		return ErrMismatchValidatorsType
	}

	for idx := 0; idx < ss.Len(); idx++ {
		newVal := ss.At(uint64(idx))

		if s.Includes(newVal.Addr()) {
			continue
		}

		if err := s.Add(newVal); err != nil {
			return err
		}
	}

	return nil
}

// MarshalRLPWith is a RLP Marshaller
func (s *Set) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	for _, v := range s.Validators {
		vv.Set(v.MarshalRLPWith(arena))
	}

	return vv
}

// UnmarshalRLPFrom is a RLP Unmarshaller
func (s *Set) UnmarshalRLPFrom(p *fastrlp.Parser, val *fastrlp.Value) error {
	elems, err := val.GetElems()
	if err != nil {
		return err
	}

	s.Validators = make([]Validator, len(elems))

	for idx, e := range elems {
		if s.Validators[idx], err = NewValidatorFromType(s.ValidatorType); err != nil {
			return err
		}

		if err := s.Validators[idx].UnmarshalRLPFrom(p, e); err != nil {
			return err
		}
	}

	return nil
}

// Marshal implements json marshal function
func (s *Set) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Validators)
}

// UnmarshalJSON implements json unmarshal function
func (s *Set) UnmarshalJSON(data []byte) error {
	var (
		rawValidators = []json.RawMessage{}
		err           error
	)

	if err = json.Unmarshal(data, &rawValidators); err != nil {
		return err
	}

	validators := make([]Validator, len(rawValidators))

	for idx := range validators {
		if validators[idx], err = NewValidatorFromType(s.ValidatorType); err != nil {
			return err
		}

		if err := json.Unmarshal(rawValidators[idx], validators[idx]); err != nil {
			return err
		}
	}

	s.Validators = validators

	return nil
}
