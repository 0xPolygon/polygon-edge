package validators

import (
	"encoding/json"

	"github.com/0xPolygon/polygon-edge/types"
)

func (vs *ECDSAValidators) MarshalJSON() ([]byte, error) {
	addrs := make([]types.Address, len(*vs))

	for idx, v := range *vs {
		addrs[idx] = v.Address
	}

	return json.Marshal(addrs)
}

func (vs *ECDSAValidators) UnmarshalJSON(data []byte) error {
	addrs := make([]types.Address, len(*vs))

	if err := json.Unmarshal(data, &addrs); err != nil {
		return err
	}

	*vs = ECDSAValidators{}
	for _, addr := range addrs {
		if err := vs.Add(NewECDSAValidator(addr)); err != nil {
			return err
		}
	}

	return nil
}

func (vs *BLSValidators) MarshalJSON() ([]byte, error) {
	validators := make([]string, len(*vs))

	for idx, v := range *vs {
		validators[idx] = v.String()
	}

	return json.Marshal(validators)
}

func (vs *BLSValidators) UnmarshalJSON(data []byte) error {
	rawVals := make([]string, len(*vs))

	if err := json.Unmarshal(data, &rawVals); err != nil {
		return err
	}

	*vs = BLSValidators{}

	for _, rawVal := range rawVals {
		val, err := ParseBLSValidator(rawVal)
		if err != nil {
			return err
		}

		if err := vs.Add(val); err != nil {
			return err
		}
	}

	return nil
}
