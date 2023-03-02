package genesis

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"

	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	manifestPathFlag     = "manifest"
	validatorSetSizeFlag = "validator-set-size"
	sprintSizeFlag       = "sprint-size"
	blockTimeFlag        = "block-time"
	bridgeFlag           = "bridge-json-rpc"

	defaultManifestPath     = "./manifest.json"
	defaultEpochSize        = uint64(10)
	defaultSprintSize       = uint64(5)
	defaultValidatorSetSize = 100
	defaultBlockTime        = 2 * time.Second
	defaultBridge           = false
	defaultEpochReward      = 1

	bootnodePortStart = 30301
)

var (
	errNoGenesisValidators = errors.New("genesis validators aren't provided")
)

// generatePolyBftChainConfig creates and persists polybft chain configuration to the provided file path
func (p *genesisParams) generatePolyBftChainConfig() error {
	// load manifest file
	manifest, err := polybft.LoadManifest(p.manifestPath)
	if err != nil {
		return fmt.Errorf("failed to load manifest file from provided path '%s': %w", p.manifestPath, err)
	}

	if len(manifest.GenesisValidators) == 0 {
		return errNoGenesisValidators
	}

	var bridge *polybft.BridgeConfig

	// populate bridge configuration
	if p.bridgeJSONRPCAddr != "" && manifest.RootchainConfig != nil {
		bridge = manifest.RootchainConfig.ToBridgeConfig()
		bridge.JSONRPCEndpoint = p.bridgeJSONRPCAddr
	}

	polyBftConfig := &polybft.PolyBFTConfig{
		InitialValidatorSet: manifest.GenesisValidators,
		BlockTime:           p.blockTime,
		EpochSize:           p.epochSize,
		SprintSize:          p.sprintSize,
		EpochReward:         p.epochReward,
		// use 1st account as governance address
		Governance:        manifest.GenesisValidators[0].Address,
		Bridge:            bridge,
		ValidatorSetAddr:  contracts.ValidatorSetContract,
		StateReceiverAddr: contracts.StateReceiverContract,
	}

	chainConfig := &chain.Chain{
		Name: p.name,
		Params: &chain.Params{
			ChainID: int64(p.chainID),
			Forks:   chain.AllForksEnabled,
			Engine: map[string]interface{}{
				string(server.PolyBFTConsensus): polyBftConfig,
			},
		},
		Bootnodes: p.bootnodes,
	}

	premineInfos := make([]*premineInfo, len(manifest.GenesisValidators))
	validatorPreminesMap := make(map[types.Address]int, len(manifest.GenesisValidators))
	totalStake := big.NewInt(0)

	for i, validator := range manifest.GenesisValidators {
		// populate premine info for validator accounts
		premineInfo := &premineInfo{address: validator.Address, balance: validator.Balance}
		premineInfos[i] = premineInfo
		validatorPreminesMap[premineInfo.address] = i

		// TODO: @Stefan-Ethernal change this to Stake when https://github.com/0xPolygon/polygon-edge/pull/1137 gets merged
		// increment total stake
		totalStake.Add(totalStake, validator.Balance)
	}

	// deploy genesis contracts
	allocs, err := p.deployContracts(totalStake)
	if err != nil {
		return err
	}

	// either premine non-validator or override validator accounts balance
	for _, premine := range p.premine {
		premineInfo, err := parsePremineInfo(premine)
		if err != nil {
			return err
		}

		if i, ok := validatorPreminesMap[premineInfo.address]; ok {
			premineInfos[i] = premineInfo
		} else {
			premineInfos = append(premineInfos, premineInfo) //nolint:makezero
		}
	}

	// premine accounts
	for _, premine := range premineInfos {
		allocs[premine.address] = &chain.GenesisAccount{
			Balance: premine.balance,
		}
	}

	validatorMetadata := make([]*polybft.ValidatorMetadata, len(manifest.GenesisValidators))

	for i, validator := range manifest.GenesisValidators {
		// update balance of genesis validator, because it could be changed via premine flag
		balance, err := chain.GetGenesisAccountBalance(validator.Address, allocs)
		if err != nil {
			return err
		}

		validator.Balance = balance

		// create validator metadata instance
		metadata, err := validator.ToValidatorMetadata()
		if err != nil {
			return err
		}

		validatorMetadata[i] = metadata

		// set genesis validators as boot nodes if boot nodes not provided via CLI
		if len(p.bootnodes) == 0 {
			bootNodeMultiAddr := fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", "127.0.0.1", bootnodePortStart+i, validator.NodeID)
			chainConfig.Bootnodes = append(chainConfig.Bootnodes, bootNodeMultiAddr)
		}
	}

	genesisExtraData, err := generateExtraDataPolyBft(validatorMetadata)
	if err != nil {
		return err
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

	return helper.WriteGenesisConfigToDisk(chainConfig, params.genesisPath)
}

func (p *genesisParams) deployContracts(totalStake *big.Int) (map[types.Address]*chain.GenesisAccount, error) {
	genesisContracts := []struct {
		artifact *artifact.Artifact
		address  types.Address
	}{
		{
			// ChildValidatorSet contract
			artifact: contractsapi.ChildValidatorSet,
			address:  contracts.ValidatorSetContract,
		},
		{
			// State receiver contract
			artifact: contractsapi.StateReceiver,
			address:  contracts.StateReceiverContract,
		},
		{
			// NativeERC20 Token contract
			artifact: contractsapi.NativeERC20,
			address:  contracts.NativeERC20TokenContract,
		},
		{
			// ChildERC20 token contract
			artifact: contractsapi.ChildERC20,
			address:  contracts.ChildERC20Contract,
		},
		{
			// ChildERC20Predicate contract
			artifact: contractsapi.ChildERC20Predicate,
			address:  contracts.ChildERC20PredicateContract,
		},
		{
			// BLS contract
			artifact: contractsapi.BLS,
			address:  contracts.BLSContract,
		},
		{
			// Merkle contract
			artifact: contractsapi.Merkle,
			address:  contracts.MerkleContract,
		},
		{
			// L2StateSender contract
			artifact: contractsapi.L2StateSender,
			address:  contracts.L2StateSenderContract,
		},
	}

	allocations := make(map[types.Address]*chain.GenesisAccount, len(genesisContracts))

	for _, contract := range genesisContracts {
		allocations[contract.address] = &chain.GenesisAccount{
			Balance: big.NewInt(0),
			Code:    contract.artifact.DeployedBytecode,
		}
	}

	// ChildValidatorSet must have funds pre-allocated, because of withdrawal workflow
	allocations[contracts.ValidatorSetContract].Balance = totalStake

	return allocations, nil
}

// generateExtraDataPolyBft populates Extra with specific fields required for polybft consensus protocol
func generateExtraDataPolyBft(validators []*polybft.ValidatorMetadata) ([]byte, error) {
	delta := &polybft.ValidatorSetDelta{
		Added:   validators,
		Removed: bitmap.Bitmap{},
	}

	// Order validators based on its addresses
	sort.Slice(delta.Added, func(i, j int) bool {
		return bytes.Compare(delta.Added[i].Address[:], delta.Added[j].Address[:]) < 0
	})

	extra := polybft.Extra{Validators: delta, Checkpoint: &polybft.CheckpointData{}}

	return append(make([]byte, polybft.ExtraVanity), extra.MarshalRLPTo(nil)...), nil
}
