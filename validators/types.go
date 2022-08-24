package validators

import (
	"errors"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

var (
	ErrInvalidValidatorType   = errors.New("invalid validator type")
	ErrMismatchValidatorType  = errors.New("mismatch between validator and validators")
	ErrMismatchValidatorsType = errors.New("mismatch between two validators")
	ErrValidatorAlreadyExists = errors.New("validator already exists in validators")
	ErrValidatorNotFound      = errors.New("validator not found in validators")
	ErrInvalidValidators      = errors.New("container is not ")
)

type ValidatorType string

const (
	ECDSAValidatorType ValidatorType = "ecdsa"
	BLSValidatorType   ValidatorType = "bls"
)

// validatorTypes is the map used for easy string -> ValidatorType lookups
var validatorTypes = map[string]ValidatorType{
	string(ECDSAValidatorType): ECDSAValidatorType,
	string(BLSValidatorType):   BLSValidatorType,
}

// ParseValidatorType converts a validatorType string representation to a ValidatorType
func ParseValidatorType(validatorType string) (ValidatorType, error) {
	// Check if the cast is possible
	castType, ok := validatorTypes[validatorType]
	if !ok {
		return castType, ErrInvalidValidatorType
	}

	return castType, nil
}

// Validator defines the interface of the methods a validator implements
type Validator interface {
	// Return the validator type
	Type() ValidatorType
	// Return the string representation
	String() string
	// Return the address of the validator
	Addr() types.Address
	// Return of copy of the validator
	Copy() Validator
	// Check the same validator or not
	Equal(Validator) bool
	// RLP Marshaller to encode to bytes
	MarshalRLPWith(*fastrlp.Arena) *fastrlp.Value
	// RLP Unmarshaller to encode from bytes
	UnmarshalRLPFrom(*fastrlp.Parser, *fastrlp.Value) error
	// Return bytes in RLP encode
	Bytes() []byte
	// Decode bytes in RLP encode and map to the fields
	SetFromBytes([]byte) error
}

// Validators defines the interface of the methods validator collection implements
type Validators interface {
	// Return the type of the validators
	Type() ValidatorType
	// Return the size of collection
	Len() int
	// Check equality of each element
	Equal(Validators) bool
	// Return of the whole collection
	Copy() Validators
	// Get validator at specified height
	At(uint64) Validator
	// Find the index of the validator that has specified address
	Index(types.Address) int64
	// Check the validator that has specified address exists in the collection
	Includes(types.Address) bool
	// Add a validator into collection
	Add(Validator) error
	// Remove a validator from collection
	Del(Validator) error
	// Merge 2 collections into one collection
	Merge(Validators) error
	// RLP Marshaller to encode to bytes
	MarshalRLPWith(*fastrlp.Arena) *fastrlp.Value
	// Decode bytes in RLP encode and map to the elements
	UnmarshalRLPFrom(*fastrlp.Parser, *fastrlp.Value) error
}
