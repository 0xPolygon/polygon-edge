package validators

import (
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
	return arena.NewBytes(v.Address.Bytes())
}

// UnmarshalRLPFrom is a RLP Unmarshaller
func (v *ECDSAValidator) UnmarshalRLPFrom(p *fastrlp.Parser, val *fastrlp.Value) error {
	return val.GetAddr(v.Address[:])
}

// Bytes returns bytes of ECDSAValidator
func (v *ECDSAValidator) Bytes() []byte {
	return v.Address.Bytes()
}

// SetFromBytes parses given bytes
func (v *ECDSAValidator) SetFromBytes(input []byte) error {
	v.Address = types.BytesToAddress(input)

	return nil
}
