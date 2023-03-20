package genesis

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
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
	manifestPathFlag       = "manifest"
	validatorSetSizeFlag   = "validator-set-size"
	sprintSizeFlag         = "sprint-size"
	blockTimeFlag          = "block-time"
	bridgeFlag             = "bridge-json-rpc"
	trackerStartBlocksFlag = "tracker-start-blocks"
	trieRootFlag           = "trieroot"

	defaultManifestPath     = "./manifest.json"
	defaultEpochSize        = uint64(10)
	defaultSprintSize       = uint64(5)
	defaultValidatorSetSize = 100
	defaultBlockTime        = 2 * time.Second
	defaultBridge           = false
	defaultEpochReward      = 1

	contractDeployedAllowListAdminFlag   = "contract-deployer-allow-list-admin"
	contractDeployedAllowListEnabledFlag = "contract-deployer-allow-list-enabled"

	bootnodePortStart = 30301
)

var (
	errNoGenesisValidators = errors.New("genesis validators aren't provided")
)

// generatePolyBftChainConfig creates and persists polybft chain configuration to the provided file path
func (p *genesisParams) generatePolyBftChainConfig(o command.OutputFormatter) error {
	// load manifest file
	manifest, err := polybft.LoadManifest(p.manifestPath)
	if err != nil {
		return fmt.Errorf("failed to load manifest file from provided path '%s': %w", p.manifestPath, err)
	}

	if len(manifest.GenesisValidators) == 0 {
		return errNoGenesisValidators
	}

	eventTrackerStartBlock, err := parseTrackerStartBlocks(params.eventTrackerStartBlocks)
	if err != nil {
		return err
	}

	var bridge *polybft.BridgeConfig

	// populate bridge configuration
	if p.bridgeJSONRPCAddr != "" && manifest.RootchainConfig != nil {
		bridge = manifest.RootchainConfig.ToBridgeConfig()
		bridge.JSONRPCEndpoint = p.bridgeJSONRPCAddr
		bridge.EventTrackerStartBlocks = eventTrackerStartBlock
	}

	if _, err := o.Write([]byte("[GENESIS VALIDATORS]\n")); err != nil {
		return err
	}

	for _, v := range manifest.GenesisValidators {
		if _, err := o.Write([]byte(fmt.Sprintf("%v\n", v))); err != nil {
			return err
		}
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
		InitialTrieRoot:   types.StringToHash(p.initialStateRoot),
	}

	chainConfig := &chain.Chain{
		Name: p.name,
		Params: &chain.Params{
			ChainID: manifest.ChainID,
			Forks:   chain.AllForksEnabled,
			Engine: map[string]interface{}{
				string(server.PolyBFTConsensus): polyBftConfig,
			},
		},
		Bootnodes: p.bootnodes,
	}

	genesisValidators := make(map[types.Address]struct{}, len(manifest.GenesisValidators))
	totalStake := big.NewInt(0)

	for _, validator := range manifest.GenesisValidators {
		// populate premine info for validator accounts
		genesisValidators[validator.Address] = struct{}{}

		// TODO: @Stefan-Ethernal change this to Stake when https://github.com/0xPolygon/polygon-edge/pull/1137 gets merged
		// increment total stake
		totalStake.Add(totalStake, validator.Balance)
	}

	// deploy genesis contracts
	allocs, err := p.deployContracts(totalStake)
	if err != nil {
		return err
	}

	premineInfos := make([]*PremineInfo, len(p.premine))
	premineValidatorsAddrs := []string{}
	// premine non-validator
	for i, premine := range p.premine {
		premineInfo, err := ParsePremineInfo(premine)
		if err != nil {
			return err
		}

		// collect validators addresses which got premined, as it is an error
		// genesis validators balances must be defined in manifest file and should not be changed in the genesis
		if _, ok := genesisValidators[premineInfo.Address]; ok {
			premineValidatorsAddrs = append(premineValidatorsAddrs, premineInfo.Address.String())
		} else {
			premineInfos[i] = premineInfo
		}
	}

	// if there are any premined validators in the genesis command, consider it as an error
	if len(premineValidatorsAddrs) > 0 {
		return fmt.Errorf("it is not allowed to override genesis validators balance outside from the manifest definition. "+
			"Validators which got premined: (%s)", strings.Join(premineValidatorsAddrs, ", "))
	}

	// populate genesis validators balances
	for _, validator := range manifest.GenesisValidators {
		allocs[validator.Address] = &chain.GenesisAccount{
			Balance: validator.Balance,
		}
	}

	// premine non-validator accounts
	for _, premine := range premineInfos {
		allocs[premine.Address] = &chain.GenesisAccount{
			Balance: premine.Balance,
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
			chainConfig.Bootnodes = append(chainConfig.Bootnodes, validator.MultiAddr)
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

	if len(p.contractDeployerAllowListAdmin) != 0 {
		// only enable allow list if there is at least one address as **admin**, otherwise
		// the allow list could never be updated
		chainConfig.Params.ContractDeployerAllowList = &chain.AllowListConfig{
			AdminAddresses:   stringSliceToAddressSlice(p.contractDeployerAllowListAdmin),
			EnabledAddresses: stringSliceToAddressSlice(p.contractDeployerAllowListEnabled),
		}
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

	extra := polybft.Extra{Validators: delta, Checkpoint: &polybft.CheckpointData{}}

	return append(make([]byte, polybft.ExtraVanity), extra.MarshalRLPTo(nil)...), nil
}

func stringSliceToAddressSlice(addrs []string) []types.Address {
	res := make([]types.Address, len(addrs))
	for indx, addr := range addrs {
		res[indx] = types.StringToAddress(addr)
	}

	return res
}
