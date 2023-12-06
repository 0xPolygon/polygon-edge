package genesis

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	dirFlag                      = "dir"
	nameFlag                     = "name"
	premineFlag                  = "premine"
	stakeFlag                    = "stake"
	chainIDFlag                  = "chain-id"
	epochSizeFlag                = "epoch-size"
	epochRewardFlag              = "epoch-reward"
	blockGasLimitFlag            = "block-gas-limit"
	genesisBaseFeeConfigFlag     = "base-fee-config"
	nativeTokenConfigFlag        = "native-token-config"
	rewardTokenCodeFlag          = "reward-token-code"
	rewardWalletFlag             = "reward-wallet"
	blockTrackerPollIntervalFlag = "block-tracker-poll-interval"
	proxyContractsAdminFlag      = "proxy-contracts-admin"
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
	errRewardWalletZero         = errors.New("reward wallet address must not be zero address")
)

type genesisParams struct {
	genesisPath  string
	name         string
	consensusRaw string
	premine      []string
	stake        []string
	bootnodes    []string

	chainID   uint64
	epochSize uint64

	blockGasLimit uint64

	baseFeeConfig       string
	parsedBaseFeeConfig *baseFeeInfo

	minNumValidators     uint64
	maxNumValidators     uint64
	validatorsPath       string
	validatorsPrefixPath string
	validators           []string

	extraData []byte

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
	stakeInfos   map[types.Address]*big.Int

	// rewards
	rewardTokenCode string
	rewardWallet    string

	blockTrackerPollInterval time.Duration

	proxyContractsAdmin string
	bladeAdmin          string
}

func (p *genesisParams) validateFlags() error {
	// Check if the consensusRaw is supported
	if !server.ConsensusSupported(p.consensusRaw) {
		return errUnsupportedConsensus
	}

	// Check if the genesis file already exists
	if err := verifyGenesisExistence(p.genesisPath); err != nil {
		return errors.New(err.GetMessage())
	}

	if err := p.validateGenesisBaseFeeConfig(); err != nil {
		return err
	}

	// Check if validator information is set at all
	if !p.areValidatorsSetManually() && !p.areValidatorsSetByPrefix() {
		return errValidatorsNotSpecified
	}

	if err := p.parsePremineInfo(); err != nil {
		return err
	}

	if p.isPolyBFTConsensus() {
		if err := p.extractNativeTokenMetadata(); err != nil {
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

		if err := p.validateBladeAdminFlag(); err != nil {
			return err
		}

		if err := p.parseStakeInfo(); err != nil {
			return err
		}
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

func (p *genesisParams) isPolyBFTConsensus() bool {
	return server.ConsensusType(p.consensusRaw) == server.PolyBFTConsensus
}

func (p *genesisParams) areValidatorsSetManually() bool {
	return len(p.validators) != 0
}

func (p *genesisParams) areValidatorsSetByPrefix() bool {
	return p.validatorsPrefixPath != ""
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
	enabledForks := chain.AllForksEnabled.Copy()
	if p.parsedBaseFeeConfig == nil {
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
			Engine: map[string]interface{}{
				p.consensusRaw: map[string]interface{}{},
			},
		},
		Bootnodes: p.bootnodes,
	}

	if p.parsedBaseFeeConfig != nil {
		chainConfig.Genesis.BaseFee = p.parsedBaseFeeConfig.baseFee
		chainConfig.Genesis.BaseFeeChangeDenom = p.parsedBaseFeeConfig.baseFeeChangeDenom
		chainConfig.Genesis.BaseFeeEM = p.parsedBaseFeeConfig.baseFeeEM
	}

	chainConfig.Params.BurnContract = make(map[uint64]types.Address, 1)
	chainConfig.Params.BurnContract[0] = types.ZeroAddress

	for _, premineInfo := range p.premineInfos {
		chainConfig.Genesis.Alloc[premineInfo.Address] = &chain.GenesisAccount{
			Balance: premineInfo.Amount,
		}
	}

	p.genesisConfig = chainConfig

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

func (p *genesisParams) parseStakeInfo() error {
	p.stakeInfos = make(map[types.Address]*big.Int, len(p.stake))

	for _, stake := range p.stake {
		stakeInfo, err := helper.ParsePremineInfo(stake)
		if err != nil {
			return fmt.Errorf("invalid stake amount provided: %w", err)
		}

		p.stakeInfos[stakeInfo.Address] = stakeInfo.Amount
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

func (p *genesisParams) validateGenesisBaseFeeConfig() error {
	if p.baseFeeConfig == "" {
		return nil
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

func (p *genesisParams) getResult() command.CommandResult {
	return &GenesisResult{
		Message: fmt.Sprintf("\nGenesis written to %s\n", p.genesisPath),
	}
}
