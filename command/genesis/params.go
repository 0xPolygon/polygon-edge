package genesis

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/output"
	"github.com/0xPolygon/polygon-edge/consensus/ibft"
	"github.com/0xPolygon/polygon-edge/helper/staking"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/types"
	"io/ioutil"
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
)

var (
	params = &genesisParams{}
)

var (
	errValidatorsNotSpecified         = errors.New("validator information not specified")
	errValidatorsSpecifiedIncorrectly = errors.New("validator information specified through mutually exclusive flags")
	errUnsupportedConsensus           = errors.New("specified consensusRaw not supported")
	errMissingBootnode                = errors.New("at least 1 bootnode is required")
	errInvalidEpochSize               = errors.New("epoch size must be greater than 1")
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

	extraData []byte
	consensus server.ConsensusType

	consensusEngineConfig map[string]interface{}

	genesisConfig *chain.Chain
}

func (p *genesisParams) validateFlags() error {
	// Check if the correct number of bootnodes is provided
	if len(p.bootnodes) < 1 {
		return errMissingBootnode
	}

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

	// Check if mutually exclusive flags are set correctly
	if p.isIBFTConsensus() &&
		p.areValidatorsSetManually() &&
		p.areValidatorsSetByPrefix() {
		return errValidatorsSpecifiedIncorrectly
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

func (p *genesisParams) initValidatorSet() error {
	if len(p.ibftValidatorsRaw) != 0 {
		for _, val := range p.ibftValidatorsRaw {
			p.ibftValidators = append(
				p.ibftValidators,
				types.StringToAddress(val),
			)
		}

		return nil
	}

	var readErr error
	if p.ibftValidators, readErr = getValidatorsFromPrefixPath(
		p.validatorPrefixPath,
	); readErr != nil {
		return fmt.Errorf("failed to read from prefix: %w", readErr)
	}

	return nil
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

	if err := p.writeGenesisToDisk(); err != nil {
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

		chainConfig.Genesis.Alloc[staking.StakingSCAddress] = stakingAccount
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
	stakingAccount, predeployErr := staking.PredeployStakingSC(p.ibftValidators)
	if predeployErr != nil {
		return nil, predeployErr
	}

	return stakingAccount, nil
}

// writeGenesisToDisk writes the passed in configuration to a genesis file at the specified path
func (p *genesisParams) writeGenesisToDisk() error {
	data, err := json.MarshalIndent(p.genesisConfig, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to generate genesis: %w", err)
	}

	//nolint:gosec
	if err := ioutil.WriteFile(p.genesisPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write genesis: %w", err)
	}

	return nil
}

func (p *genesisParams) getResult() output.CommandResult {
	return &GenesisResult{
		Message: fmt.Sprintf("Genesis written to %s\n", p.genesisPath),
	}
}
