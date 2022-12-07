package genesis

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"

	rootchain "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	premineValidatorsFlag          = "premine-validators"
	polyBftValidatorPrefixPathFlag = "validator-prefix"
	smartContractsRootPathFlag     = "contracts-path"

	validatorSetSizeFlag = "validator-set-size"
	sprintSizeFlag       = "sprint-size"
	blockTimeFlag        = "block-time"
	validatorsFlag       = "polybft-validators"
	bridgeFlag           = "bridge-json-rpc"

	defaultEpochSize                  = uint64(10)
	defaultSprintSize                 = uint64(5)
	defaultValidatorSetSize           = 100
	defaultBlockTime                  = 2 * time.Second
	defaultPolyBftValidatorPrefixPath = "test-chain-"
	defaultBridge                     = false

	bootnodePortStart = 30301
)

var (
	errNoGenesisValidators = errors.New("genesis validators aren't provided")
)

func (p *genesisParams) generatePolyBFTConfig() (*chain.Chain, error) {
	// set initial validator set
	genesisValidators, err := p.getGenesisValidators()
	if err != nil {
		return nil, err
	}

	if len(genesisValidators) == 0 {
		return nil, errNoGenesisValidators
	}

	// deploy genesis contracts
	allocs, err := p.deployContracts()
	if err != nil {
		return nil, err
	}

	// premine accounts with some tokens
	var (
		validatorPreminesMap map[types.Address]int
		premineInfos         []*premineInfo
	)

	if p.premineValidators != "" {
		validatorPreminesMap = make(map[types.Address]int, len(genesisValidators))

		for i, vi := range genesisValidators {
			premineInfo, err := parsePremineInfo(fmt.Sprintf("%s:%s", vi.Address, p.premineValidators))
			if err != nil {
				return nil, err
			}

			premineInfos = append(premineInfos, premineInfo)
			validatorPreminesMap[premineInfo.address] = i
		}
	}

	for _, premine := range p.premine {
		premineInfo, err := parsePremineInfo(premine)
		if err != nil {
			return nil, err
		}

		if i, ok := validatorPreminesMap[premineInfo.address]; ok {
			premineInfos[i] = premineInfo
		} else {
			premineInfos = append(premineInfos, premineInfo)
		}
	}

	// premine accounts
	fillPremineMap(allocs, premineInfos)

	// populate genesis validators balances
	for _, validator := range genesisValidators {
		balance, err := chain.GetGenesisAccountBalance(validator.Address, allocs)
		if err != nil {
			return nil, err
		}

		validator.Balance = balance
	}

	polyBftConfig := &polybft.PolyBFTConfig{
		BlockTime:        p.blockTime,
		EpochSize:        p.epochSize,
		SprintSize:       p.sprintSize,
		ValidatorSetSize: p.validatorSetSize,
		// use 1st account as governance address
		Governance: genesisValidators[0].Address,
	}

	// populate bridge configuration
	if p.bridgeJSONRPCAddr != "" {
		polyBftConfig.Bridge = &polybft.BridgeConfig{
			// TODO: Figure out population of rootchain contracts and whether those should be part of genesis configuration
			BridgeAddr:      rootchain.StateSenderAddress,
			CheckpointAddr:  rootchain.CheckpointManagerAddress,
			JSONRPCEndpoint: p.bridgeJSONRPCAddr,
		}
	}

	chainConfig := &chain.Chain{
		Name: p.name,
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
		for i, validator := range genesisValidators {
			bootNode := fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", "127.0.0.1", bootnodePortStart+i, validator.NodeID)
			chainConfig.Bootnodes = append(chainConfig.Bootnodes, bootNode)
		}
	}

	polyBftConfig.InitialValidatorSet = genesisValidators

	genesisExtraData, err := generateExtraDataPolyBft(genesisValidators)
	if err != nil {
		return nil, err
	}

	// populate genesis parameters
	chainConfig.Genesis = &chain.Genesis{
		GasLimit:   p.blockGasLimit,
		Difficulty: 0,
		Alloc:      allocs,
		ExtraData:  genesisExtraData,
		GasUsed:    command.DefaultGenesisGasUsed,
		Mixhash:    polybft.PolyBFTMixDigest,
	}

	return chainConfig, nil
}

func (p *genesisParams) getGenesisValidators() ([]*polybft.Validator, error) {
	if len(p.validators) > 0 {
		validators := make([]*polybft.Validator, len(p.validators))
		for i, validator := range p.validators {
			parts := strings.Split(validator, ":")

			if len(parts) != 3 {
				return nil, fmt.Errorf("expected 3 parts provided in the following format <nodeId:address:blsKey>, but got %d",
					len(parts))
			}

			if len(parts[0]) != 53 {
				return nil, fmt.Errorf("invalid node id: %s", parts[0])
			}

			if len(parts[1]) != 42 {
				return nil, fmt.Errorf("invalid address: %s", parts[1])
			}

			if len(parts[2]) < 2 {
				return nil, fmt.Errorf("invalid bls key: %s", parts[2])
			}

			validators[i] = &polybft.Validator{
				NodeID:  parts[0],
				Address: types.StringToAddress(parts[1]),
				BlsKey:  parts[2],
			}
		}

		return validators, nil
	}

	return ReadValidatorsByRegexp(path.Dir(p.genesisPath), p.polyBftValidatorPrefixPath)
}

func (p *genesisParams) generatePolyBftGenesis() error {
	config, err := params.generatePolyBFTConfig()
	if err != nil {
		return err
	}

	return helper.WriteGenesisConfigToDisk(config, params.genesisPath)
}

func (p *genesisParams) deployContracts() (map[types.Address]*chain.GenesisAccount, error) {
	genesisContracts := []struct {
		name         string
		relativePath string
		address      types.Address
	}{
		{
			// Validator contract
			name:         "ChildValidatorSet",
			relativePath: "child/ChildValidatorSet.sol",
			address:      contracts.ValidatorSetContract,
		},
		{
			// State receiver contract
			name:         "StateReceiver",
			relativePath: "child/StateReceiver.sol",
			address:      contracts.StateReceiverContract,
		},
		{
			// Native Token contract (Matic ERC-20)
			name:         "MRC20",
			relativePath: "child/MRC20.sol",
			address:      contracts.NativeTokenContract,
		},
		{
			// BLS contract
			name:         "BLS",
			relativePath: "common/BLS.sol",
			address:      contracts.BLSContract,
		},
		{
			// Merkle contract
			name:         "Merkle",
			relativePath: "common/Merkle.sol",
			address:      contracts.MerkleContract,
		},
	}

	allocations := make(map[types.Address]*chain.GenesisAccount, len(genesisContracts))

	for _, contract := range genesisContracts {
		artifact, err := polybft.ReadArtifact(p.smartContractsRootPath, contract.relativePath, contract.name)
		if err != nil {
			return nil, err
		}

		allocations[contract.address] = &chain.GenesisAccount{
			Balance: big.NewInt(0),
			Code:    artifact.DeployedBytecode,
		}
	}

	return allocations, nil
}

// generateExtraDataPolyBft populates Extra with specific fields required for polybft consensus protocol
func generateExtraDataPolyBft(validators []*polybft.Validator) ([]byte, error) {
	delta := &polybft.ValidatorSetDelta{
		Added:   make(polybft.AccountSet, len(validators)),
		Removed: bitmap.Bitmap{},
	}

	for i, validator := range validators {
		blsKey, err := validator.UnmarshalBLSPublicKey()
		if err != nil {
			return nil, err
		}

		delta.Added[i] = &polybft.ValidatorMetadata{
			Address:     validator.Address,
			BlsKey:      blsKey,
			VotingPower: chain.ConvertWeiToTokensAmount(validator.Balance).Uint64(),
		}
	}

	// Order validators based on its addresses
	sort.Slice(delta.Added, func(i, j int) bool {
		return bytes.Compare(delta.Added[i].Address[:], delta.Added[j].Address[:]) < 0
	})

	extra := polybft.Extra{Validators: delta, Checkpoint: &polybft.CheckpointData{}}

	return append(make([]byte, polybft.ExtraVanity), extra.MarshalRLPTo(nil)...), nil
}
