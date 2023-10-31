package genesis

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/ibft"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/fork"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/contracts/staking"
	stakingHelper "github.com/0xPolygon/polygon-edge/helper/staking"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
)

const (
	dirFlag                      = "dir"
	nameFlag                     = "name"
	premineFlag                  = "premine"
	chainIDFlag                  = "chain-id"
	epochSizeFlag                = "epoch-size"
	epochRewardFlag              = "epoch-reward"
	blockGasLimitFlag            = "block-gas-limit"
	burnContractFlag             = "burn-contract"
	genesisBaseFeeConfigFlag     = "base-fee-config"
	posFlag                      = "pos"
	nativeTokenConfigFlag        = "native-token-config"
	rewardTokenCodeFlag          = "reward-token-code"
	rewardWalletFlag             = "reward-wallet"
	blockTrackerPollIntervalFlag = "block-tracker-poll-interval"
	proxyContractsAdminFlag      = "proxy-contracts-admin"
)

// Legacy flags that need to be preserved for running clients
const (
	chainIDFlagLEGACY = "chainid"
)

var (
	params = &genesisParams{}
)

var (
	errValidatorsNotSpecified   = errors.New("validator information not specified")
	errUnsupportedConsensus     = errors.New("specified consensusRaw not supported")
	errInvalidEpochSize         = errors.New("epoch size must be greater than 1")
	errRewardWalletAmountZero   = errors.New("reward wallet amount can not be zero or negative")
	errReserveAccMustBePremined = errors.New("it is mandatory to premine reserve account (0x0 address)")
	errBlockTrackerPollInterval = errors.New("block tracker poll interval must be greater than 0")
	errBaseFeeChangeDenomZero   = errors.New("base fee change denominator must be greater than 0")
	errBaseFeeEMZero            = errors.New("base fee elasticity multiplier must be greater than 0")
	errBaseFeeZero              = errors.New("base fee  must be greater than 0")
	errRewardWalletNotDefined   = errors.New("reward wallet address must be defined")
	errRewardTokenOnNonMintable = errors.New("a custom reward token must be defined when " +
		"native ERC20 token is non-mintable")
	errRewardWalletZero = errors.New("reward wallet address must not be zero address")
)

type genesisParams struct {
	genesisPath  string
	name         string
	consensusRaw string
	premine      []string
	bootnodes    []string

	chainID   uint64
	epochSize uint64

	blockGasLimit uint64

	burnContract        string
	baseFeeConfig       string
	parsedBaseFeeConfig *baseFeeInfo

	// PoS
	isPos                bool
	minNumValidators     uint64
	maxNumValidators     uint64
	validatorsPath       string
	validatorsPrefixPath string
	validators           []string

	// IBFT
	rawIBFTValidatorType string
	ibftValidatorType    validators.ValidatorType
	ibftValidators       validators.Validators

	extraData []byte
	consensus server.ConsensusType

	consensusEngineConfig map[string]interface{}

	genesisConfig *chain.Chain

	// PolyBFT
	sprintSize     uint64
	blockTime      time.Duration
	epochReward    uint64
	blockTimeDrift uint64

	initialStateRoot string

	// access lists
	contractDeployerAllowListAdmin   []string
	contractDeployerAllowListEnabled []string
	contractDeployerBlockListAdmin   []string
	contractDeployerBlockListEnabled []string
	transactionsAllowListAdmin       []string
	transactionsAllowListEnabled     []string
	transactionsBlockListAdmin       []string
	transactionsBlockListEnabled     []string
	bridgeAllowListAdmin             []string
	bridgeAllowListEnabled           []string
	bridgeBlockListAdmin             []string
	bridgeBlockListEnabled           []string

	nativeTokenConfigRaw string
	nativeTokenConfig    *polybft.TokenConfig

	premineInfos []*helper.PremineInfo

	// rewards
	rewardTokenCode string
	rewardWallet    string

	blockTrackerPollInterval time.Duration

	proxyContractsAdmin string
}

func (p *genesisParams) validateFlags() error {
	// Check if the consensusRaw is supported
	if !server.ConsensusSupported(p.consensusRaw) {
		return errUnsupportedConsensus
	}

	if err := p.validateGenesisBaseFeeConfig(); err != nil {
		return err
	}

	// Check if validator information is set at all
	if p.isIBFTConsensus() &&
		!p.areValidatorsSetManually() &&
		!p.areValidatorsSetByPrefix() {
		return errValidatorsNotSpecified
	}

	if err := p.parsePremineInfo(); err != nil {
		return err
	}

	if p.isPolyBFTConsensus() {
		if err := p.extractNativeTokenMetadata(); err != nil {
			return err
		}

		if err := p.validateBurnContract(); err != nil {
			return err
		}

		if err := p.validateRewardWalletAndToken(); err != nil {
			return err
		}

		if err := p.validatePremineInfo(); err != nil {
			return err
		}

		if err := p.validateProxyContractsAdmin(); err != nil {
			return err
		}
	}

	// Check if the genesis file already exists
	if generateError := verifyGenesisExistence(p.genesisPath); generateError != nil {
		return errors.New(generateError.GetMessage())
	}

	// Check that the epoch size is correct
	if p.epochSize < 2 && (p.isIBFTConsensus() || p.isPolyBFTConsensus()) {
		// Epoch size must be greater than 1, so new transactions have a chance to be added to a block.
		// Otherwise, every block would be an endblock (meaning it will not have any transactions).
		// Check is placed here to avoid additional parsing if epochSize < 2
		return errInvalidEpochSize
	}

	// Validate validatorsPath only if validators information were not provided via CLI flag
	if len(p.validators) == 0 {
		if _, err := os.Stat(p.validatorsPath); err != nil {
			return fmt.Errorf("invalid validators path ('%s') provided. Error: %w", p.validatorsPath, err)
		}
	}

	// Validate min and max validators number
	return command.ValidateMinMaxValidatorsNumber(p.minNumValidators, p.maxNumValidators)
}

func (p *genesisParams) isIBFTConsensus() bool {
	return server.ConsensusType(p.consensusRaw) == server.IBFTConsensus
}

func (p *genesisParams) isPolyBFTConsensus() bool {
	return server.ConsensusType(p.consensusRaw) == server.PolyBFTConsensus
}

func (p *genesisParams) areValidatorsSetManually() bool {
	return len(p.validators) != 0
}

func (p *genesisParams) areValidatorsSetByPrefix() bool {
	return p.validatorsPrefixPath != ""
}

func (p *genesisParams) getRequiredFlags() []string {
	if p.isIBFTConsensus() {
		return []string{
			command.BootnodeFlag,
		}
	}

	return []string{}
}

func (p *genesisParams) initRawParams() error {
	p.consensus = server.ConsensusType(p.consensusRaw)

	if p.consensus == server.PolyBFTConsensus {
		return nil
	}

	if err := p.initIBFTValidatorType(); err != nil {
		return err
	}

	if err := p.initValidatorSet(); err != nil {
		return err
	}

	p.initIBFTExtraData()
	p.initConsensusEngineConfig()

	return nil
}

// setValidatorSetFromCli sets validator set from cli command
func (p *genesisParams) setValidatorSetFromCli() error {
	if len(p.validators) == 0 {
		return nil
	}

	newValidators, err := validators.ParseValidators(p.ibftValidatorType, p.validators)
	if err != nil {
		return err
	}

	if err = p.ibftValidators.Merge(newValidators); err != nil {
		return err
	}

	return nil
}

// setValidatorSetFromPrefixPath sets validator set from prefix path
func (p *genesisParams) setValidatorSetFromPrefixPath() error {
	if !p.areValidatorsSetByPrefix() {
		return nil
	}

	validators, err := command.GetValidatorsFromPrefixPath(
		p.validatorsPath,
		p.validatorsPrefixPath,
		p.ibftValidatorType,
	)

	if err != nil {
		return fmt.Errorf("failed to read from prefix: %w", err)
	}

	if err := p.ibftValidators.Merge(validators); err != nil {
		return err
	}

	return nil
}

func (p *genesisParams) initIBFTValidatorType() error {
	var err error
	if p.ibftValidatorType, err = validators.ParseValidatorType(p.rawIBFTValidatorType); err != nil {
		return err
	}

	return nil
}

func (p *genesisParams) initValidatorSet() error {
	p.ibftValidators = validators.NewValidatorSetFromType(p.ibftValidatorType)

	// Set validator set
	// Priority goes to cli command over prefix path
	if err := p.setValidatorSetFromPrefixPath(); err != nil {
		return err
	}

	if err := p.setValidatorSetFromCli(); err != nil {
		return err
	}

	// Validate if validator number exceeds max number
	if ok := p.isValidatorNumberValid(); !ok {
		return command.ErrValidatorNumberExceedsMax
	}

	return nil
}

func (p *genesisParams) isValidatorNumberValid() bool {
	return p.ibftValidators == nil || uint64(p.ibftValidators.Len()) <= p.maxNumValidators
}

func (p *genesisParams) initIBFTExtraData() {
	if p.consensus != server.IBFTConsensus {
		return
	}

	var committedSeal signer.Seals

	switch p.ibftValidatorType {
	case validators.ECDSAValidatorType:
		committedSeal = new(signer.SerializedSeal)
	case validators.BLSValidatorType:
		committedSeal = new(signer.AggregatedSeal)
	}

	ibftExtra := &signer.IstanbulExtra{
		Validators:     p.ibftValidators,
		ProposerSeal:   []byte{},
		CommittedSeals: committedSeal,
	}

	p.extraData = make([]byte, signer.IstanbulExtraVanity)
	p.extraData = ibftExtra.MarshalRLPTo(p.extraData)
}

func (p *genesisParams) initConsensusEngineConfig() {
	if p.consensus != server.IBFTConsensus {
		p.consensusEngineConfig = map[string]interface{}{
			p.consensusRaw: map[string]interface{}{},
		}

		return
	}

	if p.isPos {
		p.initIBFTEngineMap(fork.PoS)

		return
	}

	p.initIBFTEngineMap(fork.PoA)
}

func (p *genesisParams) initIBFTEngineMap(ibftType fork.IBFTType) {
	p.consensusEngineConfig = map[string]interface{}{
		string(server.IBFTConsensus): map[string]interface{}{
			fork.KeyType:          ibftType,
			fork.KeyValidatorType: p.ibftValidatorType,
			fork.KeyBlockTime:     p.blockTime,
			ibft.KeyEpochSize:     p.epochSize,
		},
	}
}

func (p *genesisParams) generateGenesis() error {
	if err := p.initGenesisConfig(); err != nil {
		return err
	}

	if err := helper.WriteGenesisConfigToDisk(
		p.genesisConfig,
		p.genesisPath,
	); err != nil {
		return err
	}

	return nil
}

func (p *genesisParams) initGenesisConfig() error {
	// Disable london hardfork if burn contract address is not provided
	enabledForks := chain.AllForksEnabled
	if !p.isBurnContractEnabled() {
		enabledForks.RemoveFork(chain.London)
	}

	chainConfig := &chain.Chain{
		Name: p.name,
		Genesis: &chain.Genesis{
			GasLimit:   p.blockGasLimit,
			Difficulty: 1,
			Alloc:      map[types.Address]*chain.GenesisAccount{},
			ExtraData:  p.extraData,
			GasUsed:    command.DefaultGenesisGasUsed,
		},
		Params: &chain.Params{
			ChainID: int64(p.chainID),
			Forks:   enabledForks,
			Engine:  p.consensusEngineConfig,
		},
		Bootnodes: p.bootnodes,
	}

	// burn contract can be set only for non mintable native token
	if p.isBurnContractEnabled() {
		chainConfig.Genesis.BaseFee = p.parsedBaseFeeConfig.baseFee
		chainConfig.Genesis.BaseFeeEM = p.parsedBaseFeeConfig.baseFeeEM
		chainConfig.Genesis.BaseFeeChangeDenom = p.parsedBaseFeeConfig.baseFeeChangeDenom
		chainConfig.Params.BurnContract = make(map[uint64]types.Address, 1)

		burnContractInfo, err := parseBurnContractInfo(p.burnContract)
		if err != nil {
			return err
		}

		chainConfig.Params.BurnContract[burnContractInfo.BlockNumber] = burnContractInfo.Address
		chainConfig.Params.BurnContractDestinationAddress = burnContractInfo.DestinationAddress
	}

	// Predeploy staking smart contract if needed
	if p.shouldPredeployStakingSC() {
		stakingAccount, err := p.predeployStakingSC()
		if err != nil {
			return err
		}

		chainConfig.Genesis.Alloc[staking.AddrStakingContract] = stakingAccount
	}

	for _, premineInfo := range p.premineInfos {
		chainConfig.Genesis.Alloc[premineInfo.Address] = &chain.GenesisAccount{
			Balance: premineInfo.Amount,
		}
	}

	p.genesisConfig = chainConfig

	return nil
}

func (p *genesisParams) shouldPredeployStakingSC() bool {
	// If the consensus selected is IBFT / Dev and the mechanism is Proof of Stake,
	// deploy the Staking SC
	return p.isPos && (p.consensus == server.IBFTConsensus || p.consensus == server.DevConsensus)
}

func (p *genesisParams) predeployStakingSC() (*chain.GenesisAccount, error) {
	stakingAccount, predeployErr := stakingHelper.PredeployStakingSC(
		p.ibftValidators,
		stakingHelper.PredeployParams{
			MinValidatorCount: p.minNumValidators,
			MaxValidatorCount: p.maxNumValidators,
		})
	if predeployErr != nil {
		return nil, predeployErr
	}

	return stakingAccount, nil
}

// validateRewardWalletAndToken validates reward wallet flag
func (p *genesisParams) validateRewardWalletAndToken() error {
	if p.rewardWallet == "" {
		return errRewardWalletNotDefined
	}

	if !p.nativeTokenConfig.IsMintable && p.rewardTokenCode == "" {
		return errRewardTokenOnNonMintable
	}

	premineInfo, err := helper.ParsePremineInfo(p.rewardWallet)
	if err != nil {
		return err
	}

	if premineInfo.Address == types.ZeroAddress {
		return errRewardWalletZero
	}

	// If epoch rewards are enabled, reward wallet must have some amount of premine
	if p.epochReward > 0 && premineInfo.Amount.Cmp(big.NewInt(0)) < 1 {
		return errRewardWalletAmountZero
	}

	return nil
}

// parsePremineInfo parses premine flag
func (p *genesisParams) parsePremineInfo() error {
	p.premineInfos = make([]*helper.PremineInfo, 0, len(p.premine))

	for _, premine := range p.premine {
		premineInfo, err := helper.ParsePremineInfo(premine)
		if err != nil {
			return fmt.Errorf("invalid premine balance amount provided: %w", err)
		}

		p.premineInfos = append(p.premineInfos, premineInfo)
	}

	return nil
}

// validatePremineInfo validates whether reserve account (0x0 address) is premined
func (p *genesisParams) validatePremineInfo() error {
	for _, premineInfo := range p.premineInfos {
		if premineInfo.Address == types.ZeroAddress {
			// we have premine of zero address, just return
			return nil
		}
	}

	return errReserveAccMustBePremined
}

// validateBlockTrackerPollInterval validates block tracker block interval
// which can not be 0
func (p *genesisParams) validateBlockTrackerPollInterval() error {
	if p.blockTrackerPollInterval == 0 {
		return helper.ErrBlockTrackerPollInterval
	}

	return nil
}

// validateBurnContract validates burn contract. If native token is mintable,
// burn contract flag must not be set. If native token is non mintable only one burn contract
// can be set and the specified address will be used to predeploy default EIP1559 burn contract.
func (p *genesisParams) validateBurnContract() error {
	if p.isBurnContractEnabled() {
		burnContractInfo, err := parseBurnContractInfo(p.burnContract)
		if err != nil {
			return fmt.Errorf("invalid burn contract info provided: %w", err)
		}

		if p.nativeTokenConfig.IsMintable {
			if burnContractInfo.Address != types.ZeroAddress {
				return errors.New("only zero address is allowed as burn destination for mintable native token")
			}
		} else {
			if burnContractInfo.Address == types.ZeroAddress {
				return errors.New("it is not allowed to deploy burn contract to 0x0 address")
			}
		}
	}

	return nil
}

func (p *genesisParams) validateGenesisBaseFeeConfig() error {
	if p.baseFeeConfig == "" {
		return errors.New("invalid input(empty string) for genesis base fee config flag")
	}

	baseFeeInfo, err := parseBaseFeeConfig(p.baseFeeConfig)
	if err != nil {
		return fmt.Errorf("failed to parse base fee config: %w, provided value %s", err, p.baseFeeConfig)
	}

	p.parsedBaseFeeConfig = baseFeeInfo

	if baseFeeInfo.baseFee == 0 {
		return errBaseFeeZero
	}

	if baseFeeInfo.baseFeeEM == 0 {
		return errBaseFeeEMZero
	}

	if baseFeeInfo.baseFeeChangeDenom == 0 {
		return errBaseFeeChangeDenomZero
	}

	return nil
}

func (p *genesisParams) validateProxyContractsAdmin() error {
	if strings.TrimSpace(p.proxyContractsAdmin) == "" {
		return errors.New("proxy contracts admin address must be set")
	}

	proxyContractsAdminAddr := types.StringToAddress(p.proxyContractsAdmin)
	if proxyContractsAdminAddr == types.ZeroAddress {
		return errors.New("proxy contracts admin address must not be zero address")
	}

	if proxyContractsAdminAddr == contracts.SystemCaller {
		return errors.New("proxy contracts admin address must not be system caller address")
	}

	return nil
}

// isBurnContractEnabled returns true in case burn contract info is provided
func (p *genesisParams) isBurnContractEnabled() bool {
	return p.burnContract != ""
}

// extractNativeTokenMetadata parses provided native token metadata (such as name, symbol and decimals count)
func (p *genesisParams) extractNativeTokenMetadata() error {
	tokenConfig, err := polybft.ParseRawTokenConfig(p.nativeTokenConfigRaw)
	if err != nil {
		return err
	}

	p.nativeTokenConfig = tokenConfig

	return nil
}

func (p *genesisParams) getResult() command.CommandResult {
	return &GenesisResult{
		Message: fmt.Sprintf("\nGenesis written to %s\n", p.genesisPath),
	}
}
