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
