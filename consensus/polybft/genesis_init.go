package polybft

import (
	"encoding/hex"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/polybftcontracts"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

var (
	// hard code address for the sidechain (this is only being used for testing)
	ValidatorSetAddr         = ethgo.HexToAddress("0xBd770416a3345F91E4B34576cb804a576fa48EB1")
	SidechainBridgeAddr      = ethgo.HexToAddress("0x5a443704dd4B594B382c22a083e2BD3090A6feF3")
	sidechainERC20Addr       = ethgo.HexToAddress("0x47e9Fbef8C83A1714F1951F142132E6e90F5fa5D")
	SidechainERC20BridgeAddr = ethgo.HexToAddress("0x8Be503bcdEd90ED42Eff31f56199399B2b0154CA")
)

func InitGenesis(Chain *chain.Chain, initial map[types.Address]*chain.GenesisAccount) (
	map[types.Address]*chain.GenesisAccount, error) {
	config := &PolyBFTConfig{
		ValidatorSetSize: 100,
	}

	return deployContracts(config, initial)
}

func deployContracts(
	config *PolyBFTConfig, initial map[types.Address]*chain.GenesisAccount) (
	map[types.Address]*chain.GenesisAccount, error) {
	acc := map[types.Address]*chain.GenesisAccount{}
	for k, v := range initial {
		acc[k] = v
	}

	// build validator constructor input
	validatorCons := []interface{}{}
	for _, validator := range config.Genesis {
		blsKey, err := hex.DecodeString(validator.BlsKey)
		if err != nil {
			return nil, err
		}

		pubKey, err := bls.UnmarshalPublicKey(blsKey)
		if err != nil {
			return nil, err
		}

		int4, err := pubKey.ToBigInt()
		if err != nil {
			return nil, err
		}

		enc, err := abi.Encode(int4, abi.MustNewType("uint[4]"))
		if err != nil {
			return nil, err
		}

		validatorCons = append(validatorCons, map[string]interface{}{
			"ecdsa": validator.Ecdsa,
			"bls":   enc,
		})
	}

	predefinedContracts := []struct {
		name     string
		input    []interface{}
		expected ethgo.Address
		chain    string
	}{
		{
			// Validator smart contract
			name:     "Validator",
			input:    []interface{}{validatorCons, config.ValidatorSetSize},
			expected: ValidatorSetAddr,
			chain:    "child",
		},
		{
			// Bridge in the sidechain
			name:     "SidechainBridge",
			expected: SidechainBridgeAddr,
			chain:    "child",
		},
		{
			// Target ERC20 token
			name:     "MintERC20",
			expected: sidechainERC20Addr,
			chain:    "child",
		},
		{
			// Bridge wrapper for ERC20 token
			name: "ERC20Bridge",
			input: []interface{}{
				ethgo.Address(sidechainERC20Addr),
			},
			expected: SidechainERC20BridgeAddr,
			chain:    "child",
		},
	}

	// to call the init in validator smart contract we do not need much more context in the evm object
	// that is why many fields are set as default (as of now).
	for _, contract := range predefinedContracts {
		artifact, err := polybftcontracts.ReadArtifact(contract.chain, contract.name)
		if err != nil {
			return nil, err
		}
		input, err := artifact.DeployInput(contract.input)
		if err != nil {
			return nil, err
		}

		// it is important to keep the same sender so we will always have a deterministic validator address
		// note again that this is only done for testing purposes.
		acc[types.Address(contract.expected)] = &chain.GenesisAccount{
			Code: input,
		}
	}

	return acc, nil
}
