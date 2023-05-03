package genesis

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"path"
	"strings"
	"time"

	"github.com/multiformats/go-multiaddr"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	stakeFlag            = "stake"
	validatorsFlag       = "validators"
	validatorsPathFlag   = "validators-path"
	validatorsPrefixFlag = "validators-prefix"

	defaultValidatorPrefixPath = "test-chain-"

	sprintSizeFlag = "sprint-size"
	blockTimeFlag  = "block-time"
	trieRootFlag   = "trieroot"

	defaultEpochSize        = uint64(10)
	defaultSprintSize       = uint64(5)
	defaultValidatorSetSize = 100
	defaultBlockTime        = 2 * time.Second
	defaultEpochReward      = 1

	contractDeployerAllowListAdminFlag   = "contract-deployer-allow-list-admin"
	contractDeployerAllowListEnabledFlag = "contract-deployer-allow-list-enabled"
	contractDeployerBlockListAdminFlag   = "contract-deployer-block-list-admin"
	contractDeployerBlockListEnabledFlag = "contract-deployer-block-list-enabled"
	transactionsAllowListAdminFlag       = "transactions-allow-list-admin"
	transactionsAllowListEnabledFlag     = "transactions-allow-list-enabled"
	transactionsBlockListAdminFlag       = "transactions-block-list-admin"
	transactionsBlockListEnabledFlag     = "transactions-block-list-enabled"
	bridgeAllowListAdminFlag             = "bridge-allow-list-admin"
	bridgeAllowListEnabledFlag           = "bridge-allow-list-enabled"
	bridgeBlockListAdminFlag             = "bridge-block-list-admin"
	bridgeBlockListEnabledFlag           = "bridge-block-list-enabled"

	bootnodePortStart = 30301

	ecdsaAddressLength = 40
	blsKeyLength       = 256
)

var (
	errNoGenesisValidators = errors.New("genesis validators aren't provided")
)

// generatePolyBftChainConfig creates and persists polybft chain configuration to the provided file path
func (p *genesisParams) generatePolyBftChainConfig(o command.OutputFormatter) error {
	// populate premine balance map
	premineBalances := make(map[types.Address]*premineInfo, len(p.premine))

	for _, premine := range p.premine {
		premineInfo, err := parsePremineInfo(premine)
		if err != nil {
			return fmt.Errorf("invalid balance amount provided '%s' : %w", premine, err)
		}

		premineBalances[premineInfo.address] = premineInfo
	}

	walletPremineInfo, err := parsePremineInfo(p.rewardWallet)
	if err != nil {
		return fmt.Errorf("invalid reward wallet configuration provided '%s' : %w", p.rewardWallet, err)
	}

	var (
		rewardTokenByteCode []byte
		rewardTokenAddr     = contracts.NativeERC20TokenContract
	)

	if p.rewardTokenCode == "" {
		// native token is used as a reward token, and reward wallet is not a zero address
		// so we need to add that address to premine map
		premineBalances[walletPremineInfo.address] = walletPremineInfo
	} else {
		bytes, err := hex.DecodeString(p.rewardTokenCode)
		if err != nil {
			return fmt.Errorf("could not decode reward token byte code '%s' : %w", p.rewardTokenCode, err)
		}

		rewardTokenByteCode = bytes
		rewardTokenAddr = contracts.RewardTokenContract
	}

	initialValidators, err := p.getValidatorAccounts(premineBalances)
	if err != nil {
		return fmt.Errorf("failed to retrieve genesis validators: %w", err)
	}

	if len(initialValidators) == 0 {
		return errNoGenesisValidators
	}

	if _, err := o.Write([]byte("[GENESIS VALIDATORS]\n")); err != nil {
		return err
	}

	for _, v := range initialValidators {
		if _, err := o.Write([]byte(fmt.Sprintf("%v\n", v))); err != nil {
			return err
		}
	}

	// check if there are Bridge Allow List Admins and Bridge Block List Admins
	// and if there are, get the first address as the Admin
	var bridgeAllowListAdmin types.Address
	if len(p.bridgeAllowListAdmin) > 0 {
		bridgeAllowListAdmin = types.StringToAddress(p.bridgeAllowListAdmin[0])
	}

	var bridgeBlockListAdmin types.Address
	if len(p.bridgeBlockListAdmin) > 0 {
		bridgeBlockListAdmin = types.StringToAddress(p.bridgeBlockListAdmin[0])
	}

	polyBftConfig := &polybft.PolyBFTConfig{
		InitialValidatorSet: initialValidators,
		BlockTime:           common.Duration{Duration: p.blockTime},
		EpochSize:           p.epochSize,
		SprintSize:          p.sprintSize,
		EpochReward:         p.epochReward,
		// use 1st account as governance address
		Governance:           initialValidators[0].Address,
		InitialTrieRoot:      types.StringToHash(p.initialStateRoot),
		MintableNativeToken:  p.mintableNativeToken,
		NativeTokenConfig:    p.nativeTokenConfig,
		BridgeAllowListAdmin: bridgeAllowListAdmin,
		BridgeBlockListAdmin: bridgeBlockListAdmin,
		MaxValidatorSetSize:  p.maxNumValidators,
		RewardConfig: &polybft.RewardsConfig{
			TokenAddress:  rewardTokenAddr,
			WalletAddress: walletPremineInfo.address,
			WalletAmount:  walletPremineInfo.amount,
		},
	}

	// Disable london hardfork if burn contract address is not provided
	enabledForks := chain.AllForksEnabled
	if len(p.burnContracts) == 0 {
		enabledForks.London = nil
	}

	chainConfig := &chain.Chain{
		Name: p.name,
		Params: &chain.Params{
			Forks: enabledForks,
			Engine: map[string]interface{}{
				string(server.PolyBFTConsensus): polyBftConfig,
			},
		},
		Bootnodes: p.bootnodes,
	}

	totalStake := big.NewInt(0)

	for _, validator := range initialValidators {
		// increment total stake
		totalStake.Add(totalStake, validator.Stake)
	}

	// deploy genesis contracts
	allocs, err := p.deployContracts(totalStake, rewardTokenByteCode, polyBftConfig)
	if err != nil {
		return err
	}

	// premine initial validators
	for _, v := range initialValidators {
		allocs[v.Address] = &chain.GenesisAccount{
			Balance: v.Balance,
		}
	}

	// premine other accounts
	for _, premine := range premineBalances {
		// validators have already been premined, so no need to premine them again
		if _, ok := allocs[premine.address]; ok {
			continue
		}

		allocs[premine.address] = &chain.GenesisAccount{
			Balance: premine.amount,
		}
	}

	if len(p.burnContracts) > 0 {
		chainConfig.Params.BurnContract = make(map[uint64]string, len(p.burnContracts))

		for _, burnContract := range p.burnContracts {
			block, addr, err := parseBurnContractInfo(burnContract)
			if err != nil {
				return err
			}

			chainConfig.Params.BurnContract[block] = addr.String()
		}
	}

	validatorMetadata := make([]*polybft.ValidatorMetadata, len(initialValidators))

	for i, validator := range initialValidators {
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
		chainConfig.Params.ContractDeployerAllowList = &chain.AddressListConfig{
			AdminAddresses:   stringSliceToAddressSlice(p.contractDeployerAllowListAdmin),
			EnabledAddresses: stringSliceToAddressSlice(p.contractDeployerAllowListEnabled),
		}
	}

	if len(p.contractDeployerBlockListAdmin) != 0 {
		// only enable block list if there is at least one address as **admin**, otherwise
		// the block list could never be updated
		chainConfig.Params.ContractDeployerBlockList = &chain.AddressListConfig{
			AdminAddresses:   stringSliceToAddressSlice(p.contractDeployerBlockListAdmin),
			EnabledAddresses: stringSliceToAddressSlice(p.contractDeployerBlockListEnabled),
		}
	}

	if len(p.transactionsAllowListAdmin) != 0 {
		// only enable allow list if there is at least one address as **admin**, otherwise
		// the allow list could never be updated
		chainConfig.Params.TransactionsAllowList = &chain.AddressListConfig{
			AdminAddresses:   stringSliceToAddressSlice(p.transactionsAllowListAdmin),
			EnabledAddresses: stringSliceToAddressSlice(p.transactionsAllowListEnabled),
		}
	}

	if len(p.transactionsBlockListAdmin) != 0 {
		// only enable block list if there is at least one address as **admin**, otherwise
		// the block list could never be updated
		chainConfig.Params.TransactionsBlockList = &chain.AddressListConfig{
			AdminAddresses:   stringSliceToAddressSlice(p.transactionsBlockListAdmin),
			EnabledAddresses: stringSliceToAddressSlice(p.transactionsBlockListEnabled),
		}
	}

	if len(p.bridgeAllowListAdmin) != 0 {
		// only enable allow list if there is at least one address as **admin**, otherwise
		// the allow list could never be updated
		chainConfig.Params.BridgeAllowList = &chain.AddressListConfig{
			AdminAddresses:   stringSliceToAddressSlice(p.bridgeAllowListAdmin),
			EnabledAddresses: stringSliceToAddressSlice(p.bridgeAllowListEnabled),
		}
	}

	if len(p.bridgeBlockListAdmin) != 0 {
		// only enable block list if there is at least one address as **admin**, otherwise
		// the block list could never be updated
		chainConfig.Params.BridgeBlockList = &chain.AddressListConfig{
			AdminAddresses:   stringSliceToAddressSlice(p.bridgeBlockListAdmin),
			EnabledAddresses: stringSliceToAddressSlice(p.bridgeBlockListEnabled),
		}
	}

	if len(p.burnContracts) > 0 {
		// only populate base fee and base fee multiplier values if burn contract(s)
		// is provided
		chainConfig.Genesis.BaseFee = command.DefaultGenesisBaseFee
		chainConfig.Genesis.BaseFeeEM = command.DefaultGenesisBaseFeeEM
	}

	return helper.WriteGenesisConfigToDisk(chainConfig, params.genesisPath)
}

func (p *genesisParams) deployContracts(totalStake *big.Int,
	rewardTokenByteCode []byte,
	polybftConfig *polybft.PolyBFTConfig) (map[types.Address]*chain.GenesisAccount, error) {
	type contractInfo struct {
		artifact *artifact.Artifact
		address  types.Address
	}

	genesisContracts := []*contractInfo{
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
			// ChildERC721 token contract
			artifact: contractsapi.ChildERC721,
			address:  contracts.ChildERC721Contract,
		},
		{
			// ChildERC1155 contract
			artifact: contractsapi.ChildERC1155,
			address:  contracts.ChildERC1155Contract,
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
		{
			artifact: contractsapi.ValidatorSet,
			address:  contracts.ValidatorSetContract,
		},
		{
			artifact: contractsapi.RewardPool,
			address:  contracts.RewardPoolContract,
		},
	}

	if !params.mintableNativeToken {
		genesisContracts = append(genesisContracts,
			&contractInfo{artifact: contractsapi.NativeERC20, address: contracts.NativeERC20TokenContract})
	} else {
		genesisContracts = append(genesisContracts,
			&contractInfo{artifact: contractsapi.NativeERC20Mintable, address: contracts.NativeERC20TokenContract})
	}

	if len(params.bridgeAllowListAdmin) != 0 || len(params.bridgeBlockListAdmin) != 0 {
		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.ChildERC20PredicateAccessList,
				address:  contracts.ChildERC20PredicateContract,
			})

		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.ChildERC721PredicateAccessList,
				address:  contracts.ChildERC721PredicateContract,
			})

		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.ChildERC1155PredicateAccessList,
				address:  contracts.ChildERC1155PredicateContract,
			})
	} else {
		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.ChildERC20Predicate,
				address:  contracts.ChildERC20PredicateContract,
			})

		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.ChildERC721Predicate,
				address:  contracts.ChildERC721PredicateContract,
			})

		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.ChildERC1155Predicate,
				address:  contracts.ChildERC1155PredicateContract,
			})
	}

	allocations := make(map[types.Address]*chain.GenesisAccount, len(genesisContracts)+1)

	for _, contract := range genesisContracts {
		allocations[contract.address] = &chain.GenesisAccount{
			Balance: big.NewInt(0),
			Code:    contract.artifact.DeployedBytecode,
		}
	}

	if rewardTokenByteCode != nil {
		// if reward token is provided in genesis then, add it to allocations
		// to RewardTokenContract address and update polybftConfig
		allocations[contracts.RewardTokenContract] = &chain.GenesisAccount{
			Balance: big.NewInt(0),
			Code:    rewardTokenByteCode,
		}
	}

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
func (p *genesisParams) getValidatorAccounts(
	premineBalances map[types.Address]*premineInfo) ([]*polybft.Validator, error) {
	// populate validators premine info
	stakeMap := make(map[types.Address]*premineInfo, len(p.stakes))

	for _, stake := range p.stakes {
		stakeInfo, err := parsePremineInfo(stake)
		if err != nil {
			return nil, fmt.Errorf("invalid stake amount provided '%s' : %w", stake, err)
		}

		stakeMap[stakeInfo.address] = stakeInfo
	}

	if len(p.validators) > 0 {
		validators := make([]*polybft.Validator, len(p.validators))
		for i, validator := range p.validators {
			parts := strings.Split(validator, ":")
			if len(parts) != 3 {
				return nil, fmt.Errorf("expected 4 parts provided in the following format "+
					"<P2P multi address:ECDSA address:public BLS key>, but got %d part(s)",
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

			addr := types.StringToAddress(trimmedAddress)
			validators[i] = &polybft.Validator{
				MultiAddr: parts[0],
				Address:   addr,
				BlsKey:    trimmedBLSKey,
				Balance:   getPremineAmount(addr, premineBalances, command.DefaultPremineBalance),
				Stake:     getPremineAmount(addr, stakeMap, command.DefaultStake),
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
		v.Balance = getPremineAmount(v.Address, premineBalances, command.DefaultPremineBalance)
		v.Stake = getPremineAmount(v.Address, stakeMap, command.DefaultStake)
	}

	return validators, nil
}

// getPremineAmount retrieves amount from the premine map or if not provided, returns default amount
func getPremineAmount(addr types.Address, premineMap map[types.Address]*premineInfo,
	defaultAmount *big.Int) *big.Int {
	if premine, exists := premineMap[addr]; exists {
		return premine.amount
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
