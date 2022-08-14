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

// validatorTypes is the map used for easy string -> ValidatorType lookups
var validatorTypes = map[string]ValidatorType{
	string(ECDSAValidatorType): ECDSAValidatorType,
	string(BLSValidatorType):   BLSValidatorType,
}

// ParseIBFTType converts a ibftType string representation to a IBFTType
func ParseValidatorType(validatorType string) (ValidatorType, error) {
	// Check if the cast is possible
	castType, ok := validatorTypes[validatorType]
	if !ok {
		return castType, fmt.Errorf("invalid ValidatorType type %s", validatorType)
	}

	return castType, nil
}

// Validator defines the interface of the methods a validator implements
type Validator interface {
	Type() ValidatorType                                    // Return the validator type
	String() string                                         // Return the string representation
	Addr() types.Address                                    // Return the address of the validator
	Copy() Validator                                        // Return of copy of the validator
	Equal(Validator) bool                                   // Check the same validator or not
	MarshalRLPWith(*fastrlp.Arena) *fastrlp.Value           // RLP Marshaller to encode to bytes
	UnmarshalRLPFrom(*fastrlp.Parser, *fastrlp.Value) error // RLP Unmarshaller to encode from bytes
	Bytes() []byte                                          // Reeturn bytes in RLP encode
	SetFromBytes([]byte) error                              // Decode bytes in RLP encode and map to the fields
}

// Validators defines the interface of the methods validator collection implements
type Validators interface {
	Type() ValidatorType                                    // Return the type of the validators
	Len() int                                               // Return the size of collection
	Equal(Validators) bool                                  // Check equality of each element
	Copy() Validators                                       // Return of the whole collection
	At(uint64) Validator                                    // Get validator at specified height
	Index(types.Address) int64                              // Find the index of the validator that has specified address
	Includes(types.Address) bool                            // Check the validator that has specified address exists in the collection
	Add(Validator) error                                    // Add a validator into collection
	Del(Validator) error                                    // Remove a validator from collection
	Merge(Validators) error                                 // Merge 2 collections into one collection
	MarshalRLPWith(*fastrlp.Arena) *fastrlp.Value           // RLP Marshaller to encode to bytes
	UnmarshalRLPFrom(*fastrlp.Parser, *fastrlp.Value) error // Decode bytes in RLP encode and map to the elements
}
