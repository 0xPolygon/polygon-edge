package genesis

import (
	"encoding/hex"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/polybftcontracts"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	premineValidatorsFlag          = "premine-validators"
	polyBftValidatorPrefixPathFlag = "validator-prefix"

	validatorSetSizeFlag = "validator-set-size"
	sprintSizeFlag       = "sprint-size"
	blockTimeFlag        = "block-time"
	validatorsFlag       = "polybft-validators"

	defaultEpochSize        = uint64(10)
	defaultSprintSize       = uint64(5)
	defaultValidatorSetSize = 100
	defaultBlockTime        = 2 * time.Second

	defaultPolyBftValidatorPrefixPath = "test-chain-"

	bootnodePortStart = 30301
)

var (
	// hard code address for the sidechain (this is only being used for testing)
	validatorSetAddr         = types.StringToAddress("0xBd770416a3345F91E4B34576cb804a576fa48EB1")
	sidechainBridgeAddr      = types.StringToAddress("0x5a443704dd4B594B382c22a083e2BD3090A6feF3")
	sidechainERC20Addr       = types.StringToAddress("0x47e9Fbef8C83A1714F1951F142132E6e90F5fa5D")
	sidechainERC20BridgeAddr = types.StringToAddress("0x8Be503bcdEd90ED42Eff31f56199399B2b0154CA")

	contractsRootFolder = "consensus/polybft/polybftcontracts/artifacts/contracts"
)

func (p *genesisParams) generatePolyBFTConfig() (*chain.Chain, error) {
	validatorsInfo, err := ReadValidatorsByRegexp(path.Dir(p.genesisPath), p.polyBftValidatorPrefixPath)
	if err != nil {
		return nil, err
	}

	smartContracts, err := deployContracts()
	if err != nil {
		return nil, err
	}

	polyBftConfig := &polybft.PolyBFTConfig{
		// TODO: Bridge
		InitialValidatorSet: p.getGenesisValidators(validatorsInfo),
		BlockTime:           p.blockTime,
		EpochSize:           p.epochSize,
		SprintSize:          p.sprintSize,
		ValidatorSetSize:    p.validatorSetSize,
		ValidatorSetAddr:    validatorSetAddr,
		SidechainBridgeAddr: sidechainBridgeAddr,
		SmartContracts:      smartContracts,
	}

	chainConfig := &chain.Chain{
		Name: p.name,
		Genesis: &chain.Genesis{
			GasLimit:   p.blockGasLimit,
			Difficulty: 0,
			Alloc:      map[types.Address]*chain.GenesisAccount{},
			ExtraData:  generateExtraDataPolyBft(validatorsInfo),
			GasUsed:    command.DefaultGenesisGasUsed,
			Mixhash:    polybft.PolyMixDigest,
		},
		Params: &chain.Params{
			ChainID: int(p.chainID),
			Forks:   chain.AllForksEnabled,
			Engine: map[string]interface{}{
				string(server.PolyBFTConsensus): polyBftConfig,
			},
		},
		Bootnodes: p.bootnodes,
	}

	// set generic validators as bootnodes if needed
	if len(p.bootnodes) == 0 {
		for i, validator := range validatorsInfo {
			bnode := fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", "127.0.0.1", bootnodePortStart+i, validator.NodeID)
			chainConfig.Bootnodes = append(chainConfig.Bootnodes, bnode)
		}
	}

	var premine []string = nil
	if len(p.premine) > 0 {
		premine = p.premine
	} else if p.premineValidators != "" {
		premine = make([]string, len(validatorsInfo))
		for i, vi := range validatorsInfo {
			premine[i] = fmt.Sprintf("%s:%s",
				vi.Account.Ecdsa.Address().String(), p.premineValidators)
		}
	}

	// Premine accounts
	if err := fillPremineMap(chainConfig.Genesis.Alloc, premine); err != nil {
		return nil, err
	}

	return chainConfig, nil
}

func (p *genesisParams) getGenesisValidators(validators []GenesisTarget) (result []*polybft.Validator) {
	if len(p.validators) > 0 {
		for _, validator := range p.validators {
			parts := strings.Split(validator, ":")
			if len(parts) != 2 || len(parts[0]) != 32 || len(parts[1]) < 2 {
				continue
			}

			result = append(result, &polybft.Validator{
				Address: types.StringToAddress(parts[0]),
				BlsKey:  parts[1],
			})
		}
	} else {
		for _, validator := range validators {
			pubKeyMarshalled := validator.Account.Bls.PublicKey().Marshal()

			result = append(result, &polybft.Validator{
				Address: types.Address(validator.Account.Ecdsa.Address()),
				BlsKey:  hex.EncodeToString(pubKeyMarshalled),
			})
		}
	}

	return result
}

func (p *genesisParams) generatePolyBftGenesis() error {
	config, err := params.generatePolyBFTConfig()
	if err != nil {
		return err
	}

	return helper.WriteGenesisConfigToDisk(config, params.genesisPath)
}

func deployContracts() ([]polybft.SmartContract, error) {
	predefinedContracts := []struct {
		name     string
		input    []interface{}
		expected types.Address
		chain    string
	}{
		{
			// Validator smart contract
			name:     "Validator",
			expected: validatorSetAddr,
			chain:    "child",
		},
		{
			// Bridge in the sidechain
			name:     "SidechainBridge",
			expected: sidechainBridgeAddr,
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
				sidechainERC20Addr,
			},
			expected: sidechainERC20BridgeAddr,
			chain:    "child",
		},
	}

	result := make([]polybft.SmartContract, 0, len(predefinedContracts))

	for _, contract := range predefinedContracts {
		artifact, err := polybftcontracts.ReadArtifact(contractsRootFolder, contract.chain, contract.name)
		if err != nil {
			return nil, err
		}

		input, err := artifact.DeployInput(contract.input)
		if err != nil {
			return nil, err
		}

		smartContract := polybft.SmartContract{
			Address: contract.expected,
			Code:    input,
			Name:    fmt.Sprintf("%s/%s", contract.chain, contract.name),
		}

		result = append(result, smartContract)
	}

	return result, nil
}

func generateExtraDataPolyBft(validators []GenesisTarget) []byte {
	delta := &polybft.ValidatorSetDelta{
		Added:   make(polybft.AccountSet, len(validators)),
		Removed: bitmap.Bitmap{},
	}

	for i, validator := range validators {
		delta.Added[i] = &polybft.ValidatorAccount{
			Address: types.Address(validator.Account.Ecdsa.Address()),
			BlsKey:  validator.Account.Bls.PublicKey(),
		}
	}

	extra := polybft.Extra{Validators: delta}

	return append(make([]byte, 32), extra.MarshalRLPTo(nil)...)
}
