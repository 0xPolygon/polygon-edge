package validators

import "github.com/0xPolygon/polygon-edge/types"

func AddressesToECDSAValidatorSet(addrs ...types.Address) *ECDSAValidatorSet {
	set := make(ECDSAValidatorSet, len(addrs))

	for idx, addr := range addrs {
		set[idx] = &ECDSAValidator{
			Address: addr,
		}
	}

	return &set
}

func ParseECDSAValidators(ss []string) (*ECDSAValidatorSet, error) {
	set := make(ECDSAValidatorSet, len(ss))

	for idx, s := range ss {
		val, err := ParseECDSAValidator(s)
		if err != nil {
			return nil, err
		}

		set[idx] = val
	}

	return &set, nil
}

func ParseBLSValidators(ss []string) (*BLSValidatorSet, error) {
	set := make(BLSValidatorSet, len(ss))

	for idx, s := range ss {
		val, err := ParseBLSValidator(s)
		if err != nil {
			return nil, err
		}

		set[idx] = val
	}

	return &set, nil
}
