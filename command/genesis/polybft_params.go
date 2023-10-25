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
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	sprintSizeFlag = "sprint-size"
	blockTimeFlag  = "block-time"
	trieRootFlag   = "trieroot"

	blockTimeDriftFlag = "block-time-drift"

	defaultEpochSize                = uint64(10)
	defaultSprintSize               = uint64(5)
	defaultValidatorSetSize         = 100
	defaultBlockTime                = 2 * time.Second
	defaultEpochReward              = 1
	defaultBlockTimeDrift           = uint64(10)
	defaultBlockTrackerPollInterval = time.Second

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
	errNoPremineAllowed    = errors.New("native token is not mintable, so no premine is allowed " +
		"except for zero address and reward wallet if native token is used as reward token")
)

type contractInfo struct {
	artifact *artifact.Artifact
	address  types.Address
}

// generatePolyBftChainConfig creates and persists polybft chain configuration to the provided file path
func (p *genesisParams) generatePolyBftChainConfig(o command.OutputFormatter) error {
	// populate premine balance map
	premineBalances := make(map[types.Address]*helper.PremineInfo, len(p.premine))

	for _, premine := range p.premineInfos {
		premineBalances[premine.Address] = premine
	}

	walletPremineInfo, err := helper.ParsePremineInfo(p.rewardWallet)
	if err != nil {
		return fmt.Errorf("invalid reward wallet configuration provided '%s' : %w", p.rewardWallet, err)
	}

	if !p.nativeTokenConfig.IsMintable {
		// validate premine map, no premine is allowed if token is not mintable,
		// except for the reward wallet (if native token is used as reward token) and zero address
		for a := range premineBalances {
			if a != types.ZeroAddress && (p.rewardTokenCode != "" || a != walletPremineInfo.Address) {
				return errNoPremineAllowed
			}
		}
	}

	var (
		rewardTokenByteCode []byte
		rewardTokenAddr     = contracts.NativeERC20TokenContract
	)

	if p.rewardTokenCode == "" {
		// native token is used as a reward token, and reward wallet is not a zero address
		if p.epochReward > 0 {
			// epoch reward is non zero so premine reward wallet
			premineBalances[walletPremineInfo.Address] = walletPremineInfo
		}
	} else {
		bytes, err := hex.DecodeString(p.rewardTokenCode)
		if err != nil {
			return fmt.Errorf("could not decode reward token byte code '%s' : %w", p.rewardTokenCode, err)
		}

		rewardTokenByteCode = bytes
		rewardTokenAddr = contracts.RewardTokenContract
	}

	initialValidators, err := p.getValidatorAccounts()
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

	polyBftConfig := &polybft.PolyBFTConfig{
		InitialValidatorSet: initialValidators,
		BlockTime:           common.Duration{Duration: p.blockTime},
		EpochSize:           p.epochSize,
		SprintSize:          p.sprintSize,
		EpochReward:         p.epochReward,
		// use 1st account as governance address
		Governance:          types.ZeroAddress,
		InitialTrieRoot:     types.StringToHash(p.initialStateRoot),
		NativeTokenConfig:   p.nativeTokenConfig,
		MinValidatorSetSize: p.minNumValidators,
		MaxValidatorSetSize: p.maxNumValidators,
		RewardConfig: &polybft.RewardsConfig{
			TokenAddress:  rewardTokenAddr,
			WalletAddress: walletPremineInfo.Address,
			WalletAmount:  walletPremineInfo.Amount,
		},
		BlockTimeDrift:           p.blockTimeDrift,
		BlockTrackerPollInterval: common.Duration{Duration: p.blockTrackerPollInterval},
		ProxyContractsAdmin:      types.StringToAddress(p.proxyContractsAdmin),
	}

	// Disable london hardfork if burn contract address is not provided
	enabledForks := chain.AllForksEnabled
	if !p.isBurnContractEnabled() {
		enabledForks.RemoveFork(chain.London)
	}

	chainConfig := &chain.Chain{
		Name: p.name,
		Params: &chain.Params{
			ChainID: int64(p.chainID),
			Forks:   enabledForks,
			Engine: map[string]interface{}{
				string(server.PolyBFTConsensus): polyBftConfig,
			},
		},
		Bootnodes: p.bootnodes,
	}

	burnContractAddr := types.ZeroAddress

	if p.isBurnContractEnabled() {
		chainConfig.Params.BurnContract = make(map[uint64]types.Address, 1)

		burnContractInfo, err := parseBurnContractInfo(p.burnContract)
		if err != nil {
			return err
		}

		if !p.nativeTokenConfig.IsMintable {
			// burn contract can be specified on arbitrary address for non-mintable native tokens
			burnContractAddr = burnContractInfo.Address
			chainConfig.Params.BurnContract[burnContractInfo.BlockNumber] = burnContractAddr
			chainConfig.Params.BurnContractDestinationAddress = burnContractInfo.DestinationAddress
		} else {
			// burnt funds are sent to zero address when dealing with mintable native tokens
			chainConfig.Params.BurnContract[burnContractInfo.BlockNumber] = types.ZeroAddress
		}
	}

	// deploy genesis contracts
	allocs, err := p.deployContracts(rewardTokenByteCode, polyBftConfig, chainConfig, burnContractAddr)
	if err != nil {
		return err
	}

	// premine other accounts
	for _, premine := range premineBalances {
		// validators have already been premined, so no need to premine them again
		if _, ok := allocs[premine.Address]; ok {
			continue
		}

		allocs[premine.Address] = &chain.GenesisAccount{
			Balance: premine.Amount,
		}
	}

	validatorMetadata := make([]*validator.ValidatorMetadata, len(initialValidators))

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

	genesisExtraData, err := GenerateExtraDataPolyBft(validatorMetadata)
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

	if p.isBurnContractEnabled() {
		// only populate base fee and base fee multiplier values if burn contract(s)
		// is provided
		chainConfig.Genesis.BaseFee = p.parsedBaseFeeConfig.baseFee
		chainConfig.Genesis.BaseFeeEM = p.parsedBaseFeeConfig.baseFeeEM
		chainConfig.Genesis.BaseFeeChangeDenom = p.parsedBaseFeeConfig.baseFeeChangeDenom
	}

	return helper.WriteGenesisConfigToDisk(chainConfig, params.genesisPath)
}

func (p *genesisParams) deployContracts(
	rewardTokenByteCode []byte,
	polybftConfig *polybft.PolyBFTConfig,
	chainConfig *chain.Chain,
	burnContractAddr types.Address) (map[types.Address]*chain.GenesisAccount, error) {
	proxyToImplAddrMap := contracts.GetProxyImplementationMapping()
	proxyAddresses := make([]types.Address, 0, len(proxyToImplAddrMap))

	for proxyAddr := range proxyToImplAddrMap {
		proxyAddresses = append(proxyAddresses, proxyAddr)
	}

	genesisContracts := []*contractInfo{
		{
			// State receiver contract
			artifact: contractsapi.StateReceiver,
			address:  contracts.StateReceiverContractV1,
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
			address:  contracts.BLSContractV1,
		},
		{
			// Merkle contract
			artifact: contractsapi.Merkle,
			address:  contracts.MerkleContractV1,
		},
		{
			// L2StateSender contract
			artifact: contractsapi.L2StateSender,
			address:  contracts.L2StateSenderContractV1,
		},
		{
			artifact: contractsapi.ValidatorSet,
			address:  contracts.ValidatorSetContractV1,
		},
		{
			artifact: contractsapi.RewardPool,
			address:  contracts.RewardPoolContractV1,
		},
	}

	if !params.nativeTokenConfig.IsMintable {
		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.NativeERC20,
				address:  contracts.NativeERC20TokenContractV1,
			})

		// burn contract can be set only for non-mintable native token. If burn contract is set,
		// default EIP1559 contract will be deployed.
		if p.isBurnContractEnabled() {
			genesisContracts = append(genesisContracts,
				&contractInfo{
					artifact: contractsapi.EIP1559Burn,
					address:  burnContractAddr,
				})

			proxyAddresses = append(proxyAddresses, contracts.DefaultBurnContract)
		}
	} else {
		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.NativeERC20Mintable,
				address:  contracts.NativeERC20TokenContractV1,
			})
	}

	if len(params.bridgeAllowListAdmin) != 0 || len(params.bridgeBlockListAdmin) != 0 {
		// rootchain originated tokens predicates (with access lists)
		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.ChildERC20PredicateACL,
				address:  contracts.ChildERC20PredicateContractV1,
			})

		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.ChildERC721PredicateACL,
				address:  contracts.ChildERC721PredicateContractV1,
			})

		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.ChildERC1155PredicateACL,
				address:  contracts.ChildERC1155PredicateContractV1,
			})

		// childchain originated tokens predicates (with access lists)
		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.RootMintableERC20PredicateACL,
				address:  contracts.RootMintableERC20PredicateContractV1,
			})

		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.RootMintableERC721PredicateACL,
				address:  contracts.RootMintableERC721PredicateContractV1,
			})

		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.RootMintableERC1155PredicateACL,
				address:  contracts.RootMintableERC1155PredicateContractV1,
			})
	} else {
		// rootchain originated tokens predicates
		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.ChildERC20Predicate,
				address:  contracts.ChildERC20PredicateContractV1,
			})

		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.ChildERC721Predicate,
				address:  contracts.ChildERC721PredicateContractV1,
			})

		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.ChildERC1155Predicate,
				address:  contracts.ChildERC1155PredicateContractV1,
			})

		// childchain originated tokens predicates
		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.RootMintableERC20Predicate,
				address:  contracts.RootMintableERC20PredicateContractV1,
			})

		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.RootMintableERC721Predicate,
				address:  contracts.RootMintableERC721PredicateContractV1,
			})

		genesisContracts = append(genesisContracts,
			&contractInfo{
				artifact: contractsapi.RootMintableERC1155Predicate,
				address:  contracts.RootMintableERC1155PredicateContractV1,
			})
	}

	allocations := make(map[types.Address]*chain.GenesisAccount, len(genesisContracts)+1)

	if rewardTokenByteCode != nil {
		// if reward token is provided in genesis then, add it to allocations
		// to RewardTokenContract address and update Polybft config
		allocations[contracts.RewardTokenContractV1] = &chain.GenesisAccount{
			Balance: big.NewInt(0),
			Code:    rewardTokenByteCode,
		}

		proxyAddresses = append(proxyAddresses, contracts.RewardTokenContract)
	}

	genesisContracts = append(genesisContracts, getProxyContractsInfo(proxyAddresses)...)

	for _, contract := range genesisContracts {
		allocations[contract.address] = &chain.GenesisAccount{
			Balance: big.NewInt(0),
			Code:    contract.artifact.DeployedBytecode,
		}
	}

	return allocations, nil
}

// getValidatorAccounts gathers validator accounts info either from CLI or from provided local storage
func (p *genesisParams) getValidatorAccounts() ([]*validator.GenesisValidator, error) {
	// populate validators premine info
	if len(p.validators) > 0 {
		validators := make([]*validator.GenesisValidator, len(p.validators))
		for i, val := range p.validators {
			parts := strings.Split(val, ":")
			if len(parts) != 3 {
				return nil, fmt.Errorf("expected 3 parts provided in the following format "+
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
			validators[i] = &validator.GenesisValidator{
				MultiAddr: parts[0],
				Address:   addr,
				BlsKey:    trimmedBLSKey,
				Stake:     big.NewInt(0),
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

	return validators, nil
}

func stringSliceToAddressSlice(addrs []string) []types.Address {
	res := make([]types.Address, len(addrs))
	for indx, addr := range addrs {
		res[indx] = types.StringToAddress(addr)
	}

	return res
}

func getProxyContractsInfo(addresses []types.Address) []*contractInfo {
	result := make([]*contractInfo, len(addresses))

	for i, proxyAddress := range addresses {
		result[i] = &contractInfo{
			artifact: contractsapi.GenesisProxy,
			address:  proxyAddress,
		}
	}

	return result
}
