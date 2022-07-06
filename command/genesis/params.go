package genesis

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/ibft"
	"github.com/0xPolygon/polygon-edge/contracts/staking"
	stakingHelper "github.com/0xPolygon/polygon-edge/helper/staking"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	dirFlag                 = "dir"
	nameFlag                = "name"
	premineFlag             = "premine"
	chainIDFlag             = "chain-id"
	ibftValidatorFlag       = "ibft-validator"
	ibftValidatorPrefixFlag = "ibft-validators-prefix-path"
	epochSizeFlag           = "epoch-size"
	blockGasLimitFlag       = "block-gas-limit"
	posFlag                 = "pos"
	minValidatorCount       = "min-validator-count"
	maxValidatorCount       = "max-validator-count"
)

// Legacy flags that need to be preserved for running clients
const (
	chainIDFlagLEGACY = "chainid"
)

var (
	params = &genesisParams{}
)

var (
	errValidatorsNotSpecified    = errors.New("validator information not specified")
	errValidatorNumberExceedsMax = errors.New("validator number exceeds max validator number")
	errUnsupportedConsensus      = errors.New("specified consensusRaw not supported")
	errInvalidEpochSize          = errors.New("epoch size must be greater than 1")
)

type genesisParams struct {
	genesisPath         string
	name                string
	consensusRaw        string
	validatorPrefixPath string
	premine             []string
	bootnodes           []string
	ibftValidators      []types.Address

	ibftValidatorsRaw []string

	chainID       uint64
	epochSize     uint64
	blockGasLimit uint64
	isPos         bool

	minNumValidators uint64
	maxNumValidators uint64

	extraData []byte
	consensus server.ConsensusType

	consensusEngineConfig map[string]interface{}

	genesisConfig *chain.Chain
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

	// Check if the genesis file already exists
	if generateError := verifyGenesisExistence(p.genesisPath); generateError != nil {
		return errors.New(generateError.GetMessage())
	}

	// Check that the epoch size is correct
	if p.epochSize < 2 && server.ConsensusType(p.consensusRaw) == server.IBFTConsensus {
		// Epoch size must be greater than 1, so new transactions have a chance to be added to a block.
		// Otherwise, every block would be an endblock (meaning it will not have any transactions).
		// Check is placed here to avoid additional parsing if epochSize < 2
		return errInvalidEpochSize
	}

	// Validate min and max validators number
	if err := command.ValidateMinMaxValidatorsNumber(p.minNumValidators, p.maxNumValidators); err != nil {
		return err
	}

	return nil
}

func (p *genesisParams) isIBFTConsensus() bool {
	return server.ConsensusType(p.consensusRaw) == server.IBFTConsensus
}

func (p *genesisParams) areValidatorsSetManually() bool {
	return len(p.ibftValidatorsRaw) != 0
}

func (p *genesisParams) areValidatorsSetByPrefix() bool {
	return p.validatorPrefixPath != ""
}

func (p *genesisParams) getRequiredFlags() []string {
	return []string{
		command.BootnodeFlag,
	}
}

func (p *genesisParams) initRawParams() error {
	p.consensus = server.ConsensusType(p.consensusRaw)

	if err := p.initValidatorSet(); err != nil {
		return err
	}

	p.initIBFTExtraData()
	p.initConsensusEngineConfig()

	return nil
}

// setValidatorSetFromCli sets validator set from cli command
func (p *genesisParams) setValidatorSetFromCli() {
	if len(p.ibftValidatorsRaw) != 0 {
		for _, val := range p.ibftValidatorsRaw {
			p.ibftValidators = append(
				p.ibftValidators,
				types.StringToAddress(val),
			)
		}
	}
}

// setValidatorSetFromPrefixPath sets validator set from prefix path
func (p *genesisParams) setValidatorSetFromPrefixPath() error {
	var readErr error

	if !p.areValidatorsSetByPrefix() {
		return nil
	}

	if p.ibftValidators, readErr = getValidatorsFromPrefixPath(
		p.validatorPrefixPath,
	); readErr != nil {
		return fmt.Errorf("failed to read from prefix: %w", readErr)
	}

	return nil
}

func (p *genesisParams) initValidatorSet() error {
	// Set validator set
	// Priority goes to cli command over prefix path
	if err := p.setValidatorSetFromPrefixPath(); err != nil {
		return err
	}

	p.setValidatorSetFromCli()

	// Validate if validator number exceeds max number
	if ok := p.isValidatorNumberValid(); !ok {
		return errValidatorNumberExceedsMax
	}

	return nil
}

func (p *genesisParams) isValidatorNumberValid() bool {
	return uint64(len(p.ibftValidators)) <= p.maxNumValidators
}

func (p *genesisParams) initIBFTExtraData() {
	if p.consensus != server.IBFTConsensus {
		return
	}

	ibftExtra := &ibft.IstanbulExtra{
		Validators:    p.ibftValidators,
		Seal:          []byte{},
		CommittedSeal: [][]byte{},
	}

	p.extraData = make([]byte, ibft.IstanbulExtraVanity)
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
		p.initIBFTEngineMap(ibft.PoS)

		return
	}

	p.initIBFTEngineMap(ibft.PoA)
}

func (p *genesisParams) initIBFTEngineMap(mechanism ibft.MechanismType) {
	p.consensusEngineConfig = map[string]interface{}{
		string(server.IBFTConsensus): map[string]interface{}{
			"type":      mechanism,
			"epochSize": p.epochSize,
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
			ChainID: int(p.chainID),
			Forks:   chain.AllForksEnabled,
			Engine:  p.consensusEngineConfig,
		},
		Bootnodes: p.bootnodes,
	}

	// Predeploy staking smart contract if needed
	if p.shouldPredeployStakingSC() {
		stakingAccount, err := p.predeployStakingSC()
		if err != nil {
			return err
		}

		chainConfig.Genesis.Alloc[staking.AddrStakingContract] = stakingAccount
	}

	// Premine accounts
	if err := fillPremineMap(chainConfig.Genesis.Alloc, p.premine); err != nil {
		return err
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
	stakingAccount, predeployErr := stakingHelper.PredeployStakingSC(p.ibftValidators,
		stakingHelper.PredeployParams{
			MinValidatorCount: p.minNumValidators,
			MaxValidatorCount: p.maxNumValidators,
		})
	if predeployErr != nil {
		return nil, predeployErr
	}

	return stakingAccount, nil
}

func (p *genesisParams) getResult() command.CommandResult {
	return &GenesisResult{
		Message: fmt.Sprintf("Genesis written to %s\n", p.genesisPath),
	}
}
