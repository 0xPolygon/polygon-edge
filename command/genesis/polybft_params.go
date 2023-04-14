package genesis

import (
	"errors"
	"fmt"
	"math/big"
	"path"
	"strings"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/multiformats/go-multiaddr"

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
	premineValidatorsFlag = "premine-validators"
	stakeFlag             = "stake"
	validatorsFlag        = "validators"
	validatorsPathFlag    = "validators-path"
	validatorsPrefixFlag  = "validators-prefix"

	defaultValidatorPrefixPath = "test-chain-"

	sprintSizeFlag         = "sprint-size"
	blockTimeFlag          = "block-time"
	trackerStartBlocksFlag = "tracker-start-blocks"
	trieRootFlag           = "trieroot"

	defaultEpochSize        = uint64(10)
	defaultSprintSize       = uint64(5)
	defaultValidatorSetSize = 100
	defaultBlockTime        = 2 * time.Second
	defaultBridge           = false
	defaultEpochReward      = 1

	contractDeployerAllowListAdminFlag   = "contract-deployer-allow-list-admin"
	contractDeployerAllowListEnabledFlag = "contract-deployer-allow-list-enabled"
	transactionsAllowListAdminFlag       = "transactions-allow-list-admin"
	transactionsAllowListEnabledFlag     = "transactions-allow-list-enabled"

	bootnodePortStart = 30301

	ecdsaAddressLength = 40
	blsKeyLength       = 256
	blsSignatureLength = 128
)

var (
	errNoGenesisValidators = errors.New("genesis validators aren't provided")
)

// generatePolyBftChainConfig creates and persists polybft chain configuration to the provided file path
func (p *genesisParams) generatePolyBftChainConfig(o command.OutputFormatter) error {
	initialValidators, err := p.getValidatorAccounts()
	if err != nil {
		return fmt.Errorf("failed to retrieve genesis validators: %w", err)
	}

	if len(initialValidators) == 0 {
		return errNoGenesisValidators
	}

	var bridge *polybft.BridgeConfig

	if len(p.eventTrackerStartBlocks) > 0 {
		eventTrackerStartBlock, err := parseTrackerStartBlocks(p.eventTrackerStartBlocks)
		if err != nil {
			return err
		}

		bridge := &polybft.BridgeConfig{}
		// populate bridge configuration
		bridge.EventTrackerStartBlocks = eventTrackerStartBlock
	}

	if _, err := o.Write([]byte("[GENESIS VALIDATORS]\n")); err != nil {
		return err
	}

	for _, v := range initialValidators {
		if _, err := o.Write([]byte(fmt.Sprintf("%v\n", v))); err != nil {
			return err
		}
	}

	polyBftConfig := &polybft.PolyBFTConfig{
		InitialValidatorSet: initialValidators,
		BlockTime:           common.Duration{Duration: p.blockTime},
		EpochSize:           p.epochSize,
		SprintSize:          p.sprintSize,
		EpochReward:         p.epochReward,
		Bridge:              bridge,
		// use 1st account as governance address
		Governance:          initialValidators[0].Address,
		InitialTrieRoot:     types.StringToHash(p.initialStateRoot),
		MintableNativeToken: p.mintableNativeToken,
		NativeTokenConfig:   p.nativeTokenConfig,
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

	genesisValidators := make(map[types.Address]struct{}, len(initialValidators))
	totalStake := big.NewInt(0)

	for _, validator := range initialValidators {
		// populate premine info for validator accounts
		genesisValidators[validator.Address] = struct{}{}

		// increment total stake
		totalStake.Add(totalStake, validator.Stake)
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
	for _, validator := range initialValidators {
		allocs[validator.Address] = &chain.GenesisAccount{
			Balance: validator.Balance,
		}
	}

	// premine non-validator accounts
	for _, premine := range premineInfos {
		allocs[premine.Address] = &chain.GenesisAccount{
			Balance: premine.Amount,
		}
	}

	validatorMetadata := make([]*polybft.ValidatorMetadata, len(initialValidators))

	for i, validator := range initialValidators {
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

	if len(p.transactionsAllowListAdmin) != 0 {
		// only enable allow list if there is at least one address as **admin**, otherwise
		// the allow list could never be updated
		chainConfig.Params.TransactionsAllowList = &chain.AllowListConfig{
			AdminAddresses:   stringSliceToAddressSlice(p.transactionsAllowListAdmin),
			EnabledAddresses: stringSliceToAddressSlice(p.transactionsAllowListEnabled),
		}
	}

	return helper.WriteGenesisConfigToDisk(chainConfig, params.genesisPath)
}

func (p *genesisParams) deployContracts(totalStake *big.Int) (map[types.Address]*chain.GenesisAccount, error) {
	type contractInfo struct {
		artifact *artifact.Artifact
		address  types.Address
	}

	genesisContracts := []*contractInfo{
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
			// ChildERC721 token contract
			artifact: contractsapi.ChildERC721,
			address:  contracts.ChildERC721Contract,
		},
		{
			// ChildERC721Predicate token contract
			artifact: contractsapi.ChildERC721Predicate,
			address:  contracts.ChildERC721PredicateContract,
		},
		{
			// ChildERC1155 contract
			artifact: contractsapi.ChildERC1155,
			address:  contracts.ChildERC1155Contract,
		},
		{
			// ChildERC1155Predicate token contract
			artifact: contractsapi.ChildERC1155Predicate,
			address:  contracts.ChildERC1155PredicateContract,
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

	if !params.mintableNativeToken {
		genesisContracts = append(genesisContracts,
			&contractInfo{artifact: contractsapi.NativeERC20, address: contracts.NativeERC20TokenContract})
	} else {
		genesisContracts = append(genesisContracts,
			&contractInfo{artifact: contractsapi.NativeERC20Mintable, address: contracts.NativeERC20TokenContract})
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

	return extra.MarshalRLPTo(nil), nil
}

// getValidatorAccounts gathers validator accounts info either from CLI or from provided local storage
func (p *genesisParams) getValidatorAccounts() ([]*polybft.Validator, error) {
	// populate validators premine info
	premineMap := make(map[types.Address]*PremineInfo, len(p.premineValidators))
	stakeMap := make(map[types.Address]*PremineInfo, len(p.stakes))

	for _, premine := range p.premineValidators {
		premineInfo, err := ParsePremineInfo(premine)
		if err != nil {
			return nil, fmt.Errorf("invalid balance amount provided '%s' : %w", premine, err)
		}

		premineMap[premineInfo.Address] = premineInfo
	}

	for _, stake := range p.stakes {
		stakeInfo, err := ParsePremineInfo(stake)
		if err != nil {
			return nil, fmt.Errorf("invalid stake amount provided '%s' : %w", stake, err)
		}

		stakeMap[stakeInfo.Address] = stakeInfo
	}

	if len(p.validators) > 0 {
		validators := make([]*polybft.Validator, len(p.validators))
		for i, validator := range p.validators {
			parts := strings.Split(validator, ":")
			if len(parts) != 4 {
				return nil, fmt.Errorf("expected 4 parts provided in the following format "+
					"<P2P multi address:ECDSA address:public BLS key:BLS signature>, but got %d part(s)",
					len(parts))
			}

			if _, err := multiaddr.NewMultiaddr(parts[0]); err != nil {
				return nil, fmt.Errorf("invalid P2P multi address '%s' provided: %w ", parts[0], err)
			}

			trimmedAddress := strings.TrimPrefix(parts[1], "0x")
			if len(trimmedAddress) != ecdsaAddressLength {
				return nil, fmt.Errorf("invalid ECDSA address: %s", parts[1])
			}

			trimmedBLSKey := strings.TrimPrefix(parts[2], "0x")
			if len(trimmedBLSKey) != blsKeyLength {
				return nil, fmt.Errorf("invalid BLS key: %s", parts[2])
			}

			if len(parts[3]) != blsSignatureLength {
				return nil, fmt.Errorf("invalid BLS signature: %s", parts[3])
			}

			addr := types.StringToAddress(trimmedAddress)
			validators[i] = &polybft.Validator{
				MultiAddr:    parts[0],
				Address:      addr,
				BlsKey:       trimmedBLSKey,
				BlsSignature: parts[3],
				Balance:      getPremineAmount(addr, premineMap, command.DefaultPremineBalance),
				Stake:        getPremineAmount(addr, stakeMap, command.DefaultStake),
			}
		}

		return validators, nil
	}

	validatorsPath := p.validatorsPath
	if validatorsPath == "" {
		validatorsPath = path.Dir(p.genesisPath)
	}

	validators, err := ReadValidatorsByPrefix(validatorsPath, p.validatorsPrefixPath)
	if err != nil {
		return nil, err
	}

	for _, v := range validators {
		v.Balance = getPremineAmount(v.Address, premineMap, command.DefaultPremineBalance)
		v.Stake = getPremineAmount(v.Address, stakeMap, command.DefaultStake)
	}

	return validators, nil
}

// getPremineAmount retrieves amount from the premine map or if not provided, returns default amount
func getPremineAmount(addr types.Address, premineMap map[types.Address]*PremineInfo,
	defaultAmount *big.Int) *big.Int {
	if premine, exists := premineMap[addr]; exists {
		return premine.Amount
	}

	return defaultAmount
}

func stringSliceToAddressSlice(addrs []string) []types.Address {
	res := make([]types.Address, len(addrs))
	for indx, addr := range addrs {
		res[indx] = types.StringToAddress(addr)
	}

	return res
}
