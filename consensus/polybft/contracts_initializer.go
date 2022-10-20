package polybft

import (
	"encoding/hex"
	"math/big"

	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/abi"
)

var (
	initCallStaking, _ = abi.NewMethod("function initialize(" +
		"uint256 newEpochReward," +
		"uint256 newMinStake," +
		"uint256 newMinDelegation," +
		"address[] validatorAddresses," +
		"uint256[4][] validatorPubkeys," +
		"uint256[] validatorStakes," +
		"address newBls," +
		"uint256[2] newMessage," +
		"address governance)")
)

func getInitChildValidatorSetInput(validators []*Validator, governanceAddr types.Address) ([]byte, error) {
	validatorAddresses := make([]types.Address, len(validators))
	validatorPubkeys := make([][4]*big.Int, len(validators))
	validatorStakes := make([]*big.Int, len(validators))

	for i, g := range validators {
		blsKey, err := hex.DecodeString(g.BlsKey)
		if err != nil {
			return nil, err
		}

		pubKey, err := bls.UnmarshalPublicKey(blsKey)
		if err != nil {
			return nil, err
		}

		pubKeyBig, err := pubKey.ToBigInt()
		if err != nil {
			return nil, err
		}

		validatorPubkeys[i] = pubKeyBig
		validatorAddresses[i] = g.Address
		validatorStakes[i] = g.Balance
	}

	input, err := initCallStaking.Encode([]interface{}{
		// safe numbers for the test
		big.NewInt(1),
		big.NewInt(1),
		big.NewInt(1),
		validatorAddresses,
		validatorPubkeys,
		validatorStakes,
		// address of the deployed BLS contract
		contracts.BLSContract,
		buildRegisterMessage(contracts.PolyBFTRegisterMessage),
		governanceAddr,
	})
	if err != nil {
		return nil, err
	}

	return input, nil
}

func buildRegisterMessage(m string) [2]*big.Int {
	data, err := bls.MarshalMessageToBigInt([]byte(m))
	if err != nil {
		panic("error building register message: " + err.Error())
	}

	return data
}
