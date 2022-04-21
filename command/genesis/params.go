package genesis

import (
	"errors"
	"fmt"

	"github.com/dogechain-lab/jury/chain"
	"github.com/dogechain-lab/jury/command"
	"github.com/dogechain-lab/jury/command/helper"
	"github.com/dogechain-lab/jury/consensus/ibft"
	"github.com/dogechain-lab/jury/contracts/systemcontracts"
	bridgeHelper "github.com/dogechain-lab/jury/helper/bridge"
	validatorsetHelper "github.com/dogechain-lab/jury/helper/validatorset"
	vaultHelper "github.com/dogechain-lab/jury/helper/vault"
	"github.com/dogechain-lab/jury/server"
	"github.com/dogechain-lab/jury/types"
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
	validatorsetOwner       = "validatorset-owner"
	bridgeOwner             = "bridge-owner"
	bridgeSigner            = "bridge-signer"
	vaultOwner              = "vault-owner"
)

// Legacy flags that need to be preserved for running clients
const (
	chainIDFlagLEGACY = "chainid"
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

	validatorsetOwner string
	bridgeOwner       string
	bridgeSignersRaw  []string
	bridgeSigners     []types.Address
	vaultOwner        string

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

	p.initBridgeSigners()
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

	// Predeploy ValidatorSet smart contract if needed
	if p.shouldPredeployValidatorSetSC() {
		account, err := p.predeployValidatorSetSC()
		if err != nil {
			return err
		}

		chainConfig.Genesis.Alloc[systemcontracts.AddrValidatorSetContract] = account
	}

	// Predeploy bridge contract
	if bridgeAccount, err := p.predeployBridgeSC(); err != nil {
		return err
	} else {
		chainConfig.Genesis.Alloc[systemcontracts.AddrBridgeContract] = bridgeAccount
	}

	// Predeploy vault contract if needed
	if p.shouldPredeployValidatorSetSC() {
		vaultAccount, err := p.predeployVaultSC()
		if err != nil {
			return err
		}

		chainConfig.Genesis.Alloc[systemcontracts.AddrVaultContract] = vaultAccount
	}

	// Premine accounts
	if err := fillPremineMap(chainConfig.Genesis.Alloc, p.premine); err != nil {
		return err
	}

	p.genesisConfig = chainConfig

	return nil
}

func (p *genesisParams) shouldPredeployValidatorSetSC() bool {
	// If the consensus selected is IBFT / Dev and the mechanism is Proof of Stake,
	// deploy the ValidatorSet SC
	return p.isPos && (p.consensus == server.IBFTConsensus || p.consensus == server.DevConsensus)
}

func (p *genesisParams) predeployValidatorSetSC() (*chain.GenesisAccount, error) {
	account, predeployErr := validatorsetHelper.PredeploySC(
		validatorsetHelper.PredeployParams{
			Owner:      types.StringToAddress(p.validatorsetOwner),
			Validators: p.ibftValidators,
		})
	if predeployErr != nil {
		return nil, predeployErr
	}

	return account, nil
}

func (p *genesisParams) predeployVaultSC() (*chain.GenesisAccount, error) {
	return vaultHelper.PredeployVaultSC(
		vaultHelper.PredeployParams{
			Owner: types.StringToAddress(p.vaultOwner),
		},
	)
}

func (p *genesisParams) getResult() command.CommandResult {
	return &GenesisResult{
		Message: fmt.Sprintf("Genesis written to %s\n", p.genesisPath),
	}
}

func (p *genesisParams) initBridgeSigners() {
	p.bridgeSigners = make([]types.Address, 0, len(p.bridgeSignersRaw))

	for _, signer := range p.bridgeSignersRaw {
		p.bridgeSigners = append(p.bridgeSigners, types.StringToAddress(signer))
	}
}

func (p *genesisParams) predeployBridgeSC() (*chain.GenesisAccount, error) {
	return bridgeHelper.PredeployBridgeSC(
		bridgeHelper.PredeployParams{
			Owner:   types.StringToAddress(p.bridgeOwner),
			Signers: p.bridgeSigners,
		},
	)
}
