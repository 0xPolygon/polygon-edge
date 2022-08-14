package contract

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/contracts/staking"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
)

func FetchValidators(
	validatorType validators.ValidatorType,
	transition *state.Transition,
	from types.Address,
) (validators.Validators, error) {
	switch validatorType {
	case validators.ECDSAValidatorType:
		return FetchECDSAValidators(transition, from)
	case validators.BLSValidatorType:
		return FetchBLSValidators(transition, from)
	}

	return nil, fmt.Errorf("unsupported validator type: %s", validatorType)
}

func FetchECDSAValidators(
	transition *state.Transition,
	from types.Address,
) (*validators.ECDSAValidators, error) {
	valAddrs, err := staking.QueryValidators(transition, from)
	if err != nil {
		return nil, err
	}

	ecdsaValidators := &validators.ECDSAValidators{}
	for _, addr := range valAddrs {
		if err := ecdsaValidators.Add(validators.NewECDSAValidator(addr)); err != nil {
			return nil, err
		}
	}

	return ecdsaValidators, nil
}

func FetchBLSValidators(
	transition *state.Transition,
	from types.Address,
) (*validators.BLSValidators, error) {
	valAddrs, err := staking.QueryValidators(transition, from)
	if err != nil {
		return nil, err
	}

	blsPublicKeys, err := staking.QueryBLSPublicKeys(transition, from)
	if err != nil {
		return nil, err
	}

	blsValidators := &validators.BLSValidators{}

	for idx := range valAddrs {
		// ignore the validator whose BLS Key is not set
		if _, err := crypto.UnmarshalBLSPublicKey(blsPublicKeys[idx]); err != nil {
			continue
		}

		if err := blsValidators.Add(&validators.BLSValidator{
			Address:      valAddrs[idx],
			BLSPublicKey: blsPublicKeys[idx],
		}); err != nil {
			return nil, err
		}
	}

	return blsValidators, nil
}
