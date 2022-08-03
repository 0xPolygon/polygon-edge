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

// Validator defines the interface of the methods a validator implements
type Validator interface {
	Type() ValidatorType
	String() string
	Addr() types.Address
	Copy() Validator
	Equal(Validator) bool
	MarshalRLPWith(*fastrlp.Arena) *fastrlp.Value
	UnmarshalRLPFrom(*fastrlp.Parser, *fastrlp.Value) error
	Bytes() []byte
	SetFromBytes([]byte) error
}

// ValidatorSet defines the interface of the methods validator set implements
type Validators interface {
	Type() ValidatorType
	Len() int
	Equal(Validators) bool
	Copy() Validators
	At(uint64) Validator
	Index(types.Address) int64
	Includes(types.Address) bool
	Add(Validator) error
	Del(Validator) error
	Merge(Validators) error
	MarshalRLPWith(*fastrlp.Arena) *fastrlp.Value
	UnmarshalRLPFrom(*fastrlp.Parser, *fastrlp.Value) error
}
