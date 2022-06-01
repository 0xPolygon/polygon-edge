package genesis

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/ibft"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/validators"
	"github.com/0xPolygon/polygon-edge/contracts/staking"
	"github.com/0xPolygon/polygon-edge/crypto"
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
	keyTypeFlag             = "key-type"
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
	errValidatorNumberExceedsMax      = errors.New("validator number exceeds max validator number")
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
	ibftValidators      validators.ValidatorSet

	ibftValidatorsRaw []string

	chainID       uint64
	epochSize     uint64
	blockGasLimit uint64
	isPos         bool

	minNumValidators uint64
	maxNumValidators uint64

	rawKeyType string
	keyType    crypto.KeyType

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

	if err := p.initKeyType(); err != nil {
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
	if len(p.ibftValidatorsRaw) != 0 {
		var err error

		switch p.keyType {
		case crypto.KeySecp256k1:
			p.ibftValidators, err = parseECDSAValidators(p.ibftValidators, p.ibftValidatorsRaw)
		case crypto.KeyBLS:
			p.ibftValidators, err = parseBLSValidators(p.ibftValidators, p.ibftValidatorsRaw)
		}

		if err != nil {
			return err
		}
	}

	return nil
}

// setValidatorSetFromPrefixPath sets validator set from prefix path
func (p *genesisParams) setValidatorSetFromPrefixPath() error {
	var readErr error

	if !p.areValidatorsSetByPrefix() {
		return nil
	}

	if p.ibftValidators, readErr = getValidatorsFromPrefixPath(
		p.validatorPrefixPath,
		p.keyType,
	); readErr != nil {
		return fmt.Errorf("failed to read from prefix: %w", readErr)
	}

	return nil
}

func (p *genesisParams) initKeyType() error {
	key, err := crypto.ToKeyType(p.rawKeyType)
	if err != nil {
		return err
	}

	p.keyType = key

	return nil
}

func (p *genesisParams) initValidatorSet() error {
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
		return errValidatorNumberExceedsMax
	}

	return nil
}

func (p *genesisParams) isValidatorNumberValid() bool {
	return uint64(p.ibftValidators.Len()) <= p.maxNumValidators
}

func (p *genesisParams) initIBFTExtraData() {
	if p.consensus != server.IBFTConsensus {
		return
	}

	var committedSeal signer.Sealer

	switch p.keyType {
	case crypto.KeySecp256k1:
		committedSeal = new(signer.SerializedSeal)
	case crypto.KeyBLS:
		committedSeal = new(signer.BLSSeal)
	}

	ibftExtra := &signer.IstanbulExtra{
		Validators:    p.ibftValidators,
		Seal:          []byte{},
		CommittedSeal: committedSeal,
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

// stakingAccount, predeployErr := stakingHelper.PredeployStakingSC(p.ibftValidators,
// 	stakingHelper.PredeployParams{
// 		MinValidatorCount: p.minNumValidators,
// 		MaxValidatorCount: p.maxNumValidators,
// 	})
// if predeployErr != nil {
// 	return nil, predeployErr
// }

// return stakingAccount, nil
func (p *genesisParams) predeployStakingSC() (*chain.GenesisAccount, error) {
	return nil, nil
}

func (p *genesisParams) getResult() command.CommandResult {
	return &GenesisResult{
		Message: fmt.Sprintf("Genesis written to %s\n", p.genesisPath),
	}
}

func parseECDSAValidators(set validators.ValidatorSet, values []string) (validators.ValidatorSet, error) {
	addrs := make([]types.Address, 0)

	if set != nil {
		valSet, ok := set.(*validators.ECDSAValidatorSet)
		if !ok {
			return nil, fmt.Errorf("invalid validator set, expected ECDSAValidatorSet but got %T", set)
		}

		addrs = []types.Address(*valSet)
	}

	for _, v := range values {
		bytes, err := hex.DecodeString(strings.TrimPrefix(v, "0x"))
		if err != nil {
			return nil, err
		}

		addrs = append(addrs, types.BytesToAddress(bytes))
	}

	newValSet := validators.ECDSAValidatorSet(addrs)

	return &newValSet, nil
}

func parseBLSValidators(set validators.ValidatorSet, values []string) (validators.ValidatorSet, error) {
	vals := make([]validators.BLSValidator, 0)

	if set != nil {
		valSet, ok := set.(*validators.BLSValidatorSet)
		if !ok {
			return nil, fmt.Errorf("invalid validator set, expected BLSValidatorSet but got %T", set)
		}

		vals = []validators.BLSValidator(*valSet)
	}

	for _, value := range values {
		subValues := strings.Split(value, ":")

		if len(subValues) != 2 {
			return nil, fmt.Errorf("invalid validator format, expected [Validator Address]:[BLS Public Key]")
		}

		addrBytes, err := hex.DecodeString(strings.TrimPrefix(subValues[0], "0x"))
		if err != nil {
			return nil, fmt.Errorf("failed to parse address: %w", err)
		}

		pubKeyBytes, err := hex.DecodeString(strings.TrimPrefix(subValues[1], "0x"))
		if err != nil {
			return nil, fmt.Errorf("failed to parse BLS Public Key: %w", err)
		}

		vals = append(vals, validators.BLSValidator{
			Address:   types.BytesToAddress(addrBytes),
			BLSPubKey: pubKeyBytes,
		})
	}

	newValSet := validators.BLSValidatorSet(vals)

	return &newValSet, nil
}
