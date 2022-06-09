package validators

import (
	"fmt"
	"strings"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
)

func ParseECDSAValidators(values []string) (ValidatorSet, error) {
	addrs := make([]types.Address, 0)

	for _, v := range values {
		bytes, err := hex.DecodeString(strings.TrimPrefix(v, "0x"))
		if err != nil {
			return nil, err
		}

		addrs = append(addrs, types.BytesToAddress(bytes))
	}

	newValSet := ECDSAValidatorSet(addrs)

	return &newValSet, nil
}

func ParseBLSValidators(values []string) (ValidatorSet, error) {
	vals := make([]BLSValidator, 0)

	for _, value := range values {
		subValues := strings.Split(value, ":")

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

		vals = append(vals, BLSValidator{
			Address:   types.BytesToAddress(addrBytes),
			BLSPubKey: pubKeyBytes,
		})
	}

	newValSet := BLSValidatorSet(vals)

	return &newValSet, nil
}
