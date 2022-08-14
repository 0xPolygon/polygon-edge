package validators

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/0xPolygon/polygon-edge/types"
)

func NewValidatorFromType(t ValidatorType) Validator {
	switch t {
	case ECDSAValidatorType:
		return new(ECDSAValidator)
	case BLSValidatorType:
		return new(BLSValidator)
	}

	return nil
}

func NewValidatorSetFromType(t ValidatorType) Validators {
	switch t {
	case ECDSAValidatorType:
		return new(ECDSAValidators)
	case BLSValidatorType:
		return new(BLSValidators)
	}

	return nil
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

func AddressesToECDSAValidators(addrs ...types.Address) *ECDSAValidators {
	set := make(ECDSAValidators, len(addrs))

	for idx, addr := range addrs {
		set[idx] = &ECDSAValidator{
			Address: addr,
		}
	}

	return &set
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
