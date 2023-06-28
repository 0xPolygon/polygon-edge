package genesis

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/ibft"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/fork"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/contracts/staking"
	stakingHelper "github.com/0xPolygon/polygon-edge/helper/staking"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
)

const (
	dirFlag               = "dir"
	nameFlag              = "name"
	premineFlag           = "premine"
	chainIDFlag           = "chain-id"
	epochSizeFlag         = "epoch-size"
	epochRewardFlag       = "epoch-reward"
	blockGasLimitFlag     = "block-gas-limit"
	burnContractFlag      = "burn-contract"
	posFlag               = "pos"
	minValidatorCount     = "min-validator-count"
	maxValidatorCount     = "max-validator-count"
	nativeTokenConfigFlag = "native-token-config"
	rewardTokenCodeFlag   = "reward-token-code"
	rewardWalletFlag      = "reward-wallet"

	defaultNativeTokenName     = "Polygon"
	defaultNativeTokenSymbol   = "MATIC"
	defaultNativeTokenDecimals = uint8(18)
	minNativeTokenParamsNumber = 4
)

// Legacy flags that need to be preserved for running clients
const (
	chainIDFlagLEGACY = "chainid"
)

var (
	params = &genesisParams{}
)

var (
	errValidatorsNotSpecified = errors.New("validator information not specified")
	errUnsupportedConsensus   = errors.New("specified consensusRaw not supported")
	errInvalidEpochSize       = errors.New("epoch size must be greater than 1")
	errInvalidTokenParams     = errors.New("native token params were not submitted in proper format " +
		"(<name:symbol:decimals count:mintable flag:[mintable token owner address]>)")
	errRewardWalletAmountZero = errors.New("reward wallet amount can not be zero or negative")
)

type genesisParams struct {
	genesisPath         string
	name                string
	consensusRaw        string
	validatorPrefixPath string
	premine             []string
	bootnodes           []string
	ibftValidators      validators.Validators

	ibftValidatorsRaw []string

	chainID   uint64
	epochSize uint64

	blockGasLimit uint64
	isPos         bool

	burnContract string

	minNumValidators uint64
	maxNumValidators uint64

	rawIBFTValidatorType string
	ibftValidatorType    validators.ValidatorType

	extraData []byte
	consensus server.ConsensusType

	consensusEngineConfig map[string]interface{}

	genesisConfig *chain.Chain

	// PolyBFT
	validatorsPath       string
	validatorsPrefixPath string
	validators           []string
	sprintSize           uint64
	blockTime            time.Duration
	epochReward          uint64
	blockTimeDrift       uint64

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

	// rewards
	rewardTokenCode string
	rewardWallet    string
}

func (p *genesisParams) validateFlags() error {
	// Check if the consensusRaw is supported
	if !server.ConsensusSupported(p.consensusRaw) {
		return errUnsupportedConsensus
	}

	// Check if validator information is set at all
	if p.isIBFTConsensus() &&
		!p.areValidatorsSetManually() &&
		!p.areValidatorsSetByPrefix() {
		return errValidatorsNotSpecified
	}

	if p.isPolyBFTConsensus() {
		if err := p.extractNativeTokenMetadata(); err != nil {
			return err
		}

		if err := p.validateBurnContract(); err != nil {
			return err
		}

		if err := p.validateRewardWallet(); err != nil {
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
	return len(p.ibftValidatorsRaw) != 0
}

func (p *genesisParams) areValidatorsSetByPrefix() bool {
	return p.validatorPrefixPath != ""
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
	if len(p.ibftValidatorsRaw) == 0 {
		return nil
	}

	newValidators, err := validators.ParseValidators(p.ibftValidatorType, p.ibftValidatorsRaw)
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
		p.validatorPrefixPath,
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
		chainConfig.Genesis.BaseFee = command.DefaultGenesisBaseFee
		chainConfig.Genesis.BaseFeeEM = command.DefaultGenesisBaseFeeEM
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

	for _, premineRaw := range p.premine {
		premineInfo, err := parsePremineInfo(premineRaw)
		if err != nil {
			return err
		}

		chainConfig.Genesis.Alloc[premineInfo.address] = &chain.GenesisAccount{
			Balance: premineInfo.amount,
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

// validateRewardWallet validates reward wallet flag
func (p *genesisParams) validateRewardWallet() error {
	if p.rewardWallet == "" {
		return errors.New("reward wallet address must be defined")
	}

	if p.rewardWallet == types.AddressToString(types.ZeroAddress) {
		return errors.New("reward wallet address must not be zero address")
	}

	premineInfo, err := parsePremineInfo(p.rewardWallet)
	if err != nil {
		return err
	}

	if premineInfo.amount.Cmp(big.NewInt(0)) < 1 {
		return errRewardWalletAmountZero
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

// isBurnContractEnabled returns true in case burn contract info is provided
func (p *genesisParams) isBurnContractEnabled() bool {
	return p.burnContract != ""
}

// extractNativeTokenMetadata parses provided native token metadata (such as name, symbol and decimals count)
func (p *genesisParams) extractNativeTokenMetadata() error {
	if p.nativeTokenConfigRaw == "" {
		p.nativeTokenConfig = &polybft.TokenConfig{
			Name:       defaultNativeTokenName,
			Symbol:     defaultNativeTokenSymbol,
			Decimals:   defaultNativeTokenDecimals,
			IsMintable: false,
			Owner:      types.ZeroAddress,
		}

		return nil
	}

	params := strings.Split(p.nativeTokenConfigRaw, ":")
	if len(params) < minNativeTokenParamsNumber {
		return errInvalidTokenParams
	}

	// name
	name := strings.TrimSpace(params[0])
	if name == "" {
		return errInvalidTokenParams
	}

	// symbol
	symbol := strings.TrimSpace(params[1])
	if symbol == "" {
		return errInvalidTokenParams
	}

	// decimals
	decimals, err := strconv.ParseUint(strings.TrimSpace(params[2]), 10, 8)
	if err != nil || decimals > math.MaxUint8 {
		return errInvalidTokenParams
	}

	// is mintable native token used
	isMintable, err := strconv.ParseBool(strings.TrimSpace(params[3]))
	if err != nil {
		return errInvalidTokenParams
	}

	// in case it is mintable native token, it is expected to have 5 parameters provided
	if isMintable && len(params) != minNativeTokenParamsNumber+1 {
		return errInvalidTokenParams
	}

	// owner address
	owner := types.ZeroAddress
	if isMintable {
		owner = types.StringToAddress(strings.TrimSpace(params[4]))
	}

	p.nativeTokenConfig = &polybft.TokenConfig{
		Name:       name,
		Symbol:     symbol,
		Decimals:   uint8(decimals),
		IsMintable: isMintable,
		Owner:      owner,
	}

	return nil
}

func (p *genesisParams) getResult() command.CommandResult {
	return &GenesisResult{
		Message: fmt.Sprintf("\nGenesis written to %s\n", p.genesisPath),
	}
}
