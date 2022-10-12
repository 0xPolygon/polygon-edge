package genesis

import (
	"encoding/hex"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/polybftcontracts"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
)

const (
	validatorPrefixPathFlag = "prefix"

	validatorSetSizeFlag = "validator-set-size"
	sprintSizeFlag       = "sprint-size"
	blockTimeFlag        = "block-time"
	validatorsFlag       = "polybft-validators"

	defaultEpochSize        = uint64(10)
	defaultSprintSize       = uint64(5)
	defaultValidatorSetSize = 100
	defaultBlockTime        = 2 * time.Second

	bootnodePortStart = 30301
)

var (
	// hard code address for the sidechain (this is only being used for testing)
	ValidatorSetAddr         = types.StringToAddress("0xBd770416a3345F91E4B34576cb804a576fa48EB1")
	SidechainBridgeAddr      = types.StringToAddress("0x5a443704dd4B594B382c22a083e2BD3090A6feF3")
	sidechainERC20Addr       = types.StringToAddress("0x47e9Fbef8C83A1714F1951F142132E6e90F5fa5D")
	SidechainERC20BridgeAddr = types.StringToAddress("0x8Be503bcdEd90ED42Eff31f56199399B2b0154CA")
)

func (p *genesisParams) getPolyBftConfig(validators []*polybft.Validator) (*polybft.PolyBFTConfig, error) {
	smartContracts, err := deployContracts()
	if err != nil {
		return nil, err
	}

	config := &polybft.PolyBFTConfig{
		// TODO: Bridge
		InitialValidatorSet: validators,
		BlockTime:           p.blockTime,
		EpochSize:           p.epochSize,
		SprintSize:          p.sprintSize,
		ValidatorSetSize:    p.validatorSetSize,
		ValidatorSetAddr:    types.Address(ValidatorSetAddr),
		SidechainBridgeAddr: types.Address(SidechainBridgeAddr),
		SmartContracts:      smartContracts,
	}

	return config, nil
}

func (p *genesisParams) GetChainConfig() (*chain.Chain, error) {
	validatorsInfo, err := ReadValidatorsByRegexp(path.Dir(p.genesisPath), p.validatorPrefixPath)
	if err != nil {
		return nil, err
	}

	// Predeploy staking smart contracts
	genesisValidators := p.getGenesisValidators(validatorsInfo)

	polyBftConfig, err := p.getPolyBftConfig(genesisValidators)
	if err != nil {
		return nil, err
	}

	extra := polybft.Extra{Validators: GetInitialValidatorsDelta(validatorsInfo)}

	chainConfig := &chain.Chain{
		Name: p.name,
		Genesis: &chain.Genesis{
			GasLimit:   p.blockGasLimit,
			Difficulty: 0,
			Alloc:      map[types.Address]*chain.GenesisAccount{},
			ExtraData:  append(make([]byte, 32), extra.MarshalRLPTo(nil)...),
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
			// /ip4/127.0.0.1/tcp/10001/p2p/16Uiu2HAm9r5oP8Dmfsqbp1w2LdPU4YSFggKvwEmT6aTpWU8c8R13
			bnode := fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", "127.0.0.1", bootnodePortStart+i, validator.NodeID)
			chainConfig.Bootnodes = append(chainConfig.Bootnodes, bnode)
		}
	}

	// Premine accounts
	if err := fillPremineMap(chainConfig.Genesis.Alloc, p.premine); err != nil {
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
				Address: types.Address(ethgo.HexToAddress(parts[0])),
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

func deployContracts() ([]polybft.SmartContract, error) {
	predefinedContracts := []struct {
		name     string
		input    []interface{}
		expected ethgo.Address
		chain    string
	}{
		{
			// Validator smart contract
			name:     "Validator",
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
				sidechainERC20Addr,
			},
			expected: SidechainERC20BridgeAddr,
			chain:    "child",
		},
	}

	result := make([]polybft.SmartContract, 0, len(predefinedContracts))

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

		smartContract := polybft.SmartContract{
			Address: types.Address(contract.expected),
			Code:    input,
			Name:    fmt.Sprintf("%s/%s", contract.chain, contract.name),
		}

		result = append(result, smartContract)
	}

	return result, nil
}

// GetInitialValidatorsDelta extracts initial account set from the genesis block and
// populates validator set delta to its extra data
func GetInitialValidatorsDelta(validators []GenesisTarget) *polybft.ValidatorSetDelta {
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

	return delta
}
