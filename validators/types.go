package validators

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

var (
	ErrMismatchValidatorType    = errors.New("mismatch between validator and validator set")
	ErrMismatchValidatorSetType = errors.New("mismatch between validator sets")
	ErrValidatorAlreadyExists   = errors.New("validator already exists in validator set")
	ErrValidatorNotFound        = errors.New("validator not found in validator set")
)

type ValidatorType string

const (
	ECDSAValidatorType ValidatorType = "ecdsa"
	BLSValidatorType   ValidatorType = "bls"
)

func (t *ValidatorType) FromString(s string) error {
	x := ValidatorType(s)

	switch x {
	case ECDSAValidatorType, BLSValidatorType:
		*t = x

		return nil
	default:
		return fmt.Errorf("invalid validator type: %s", s)
	}
}

func NewValidatorFromType(t ValidatorType) Validator {
	switch t {
	case ECDSAValidatorType:
		return new(ECDSAValidator)
	case BLSValidatorType:
		return new(BLSValidator)
	}

	return nil
}

func NewValidatorSetFromType(t ValidatorType) ValidatorSet {
	switch t {
	case ECDSAValidatorType:
		return new(ECDSAValidatorSet)
	case BLSValidatorType:
		return new(BLSValidatorSet)
	}

	return nil
}

// Validator defines the interface of the methods a validator implements
type Validator interface {
	Addr() types.Address
	Equal(Validator) bool
	// TODO: check they're used
	MarshalRLPWith(*fastrlp.Arena) *fastrlp.Value
	UnmarshalRLPFrom(*fastrlp.Parser, *fastrlp.Value) error

	Bytes() []byte
	SetFromBytes([]byte) error
}

// ValidatorSet defines the interface of the methods validator set implements
type ValidatorSet interface {
	Type() ValidatorType
	Len() int
	Equal(ValidatorSet) bool
	Copy() ValidatorSet
	At(uint64) Validator
	Index(types.Address) int64
	Includes(types.Address) bool
	Add(Validator) error
	Del(Validator) error
	Merge(ValidatorSet) error
	MarshalRLPWith(*fastrlp.Arena) *fastrlp.Value
	UnmarshalRLPFrom(*fastrlp.Parser, *fastrlp.Value) error
}

func ParseValidator(t ValidatorType, s string) (Validator, error) {
	switch t {
	case ECDSAValidatorType:
		return ParseECDSAValidator(s)
	case BLSValidatorType:
		return ParseBLSValidator(s)
	default:
		return nil, fmt.Errorf("invalid validator type: %s", t)
	}
}
