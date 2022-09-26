package ibftswitch

import (
	"errors"
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/fork"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
)

const (
	chainFlag         = "chain"
	typeFlag          = "type"
	deploymentFlag    = "deployment"
	fromFlag          = "from"
	minValidatorCount = "min-validator-count"
	maxValidatorCount = "max-validator-count"
)

var (
	ErrFromPositive                  = errors.New(`"from" must be positive number`)
	ErrIBFTConfigNotFound            = errors.New(`"ibft" config doesn't exist in "engine" of genesis.json'`)
	ErrSameIBFTAndValidatorType      = errors.New("cannot specify same IBFT type and validator type as the last fork")
	ErrLessFromThanLastFrom          = errors.New(`"from" must be greater than the beginning height of last fork`)
	ErrInvalidValidatorsUpdateHeight = errors.New(`cannot specify a less height than 2 for validators update`)
)

var (
	params = &switchParams{}
)

type switchParams struct {
	genesisPath string
	typeRaw     string
	ibftType    fork.IBFTType

	// height
	deploymentRaw string
	deployment    *uint64
	fromRaw       string
	from          uint64

	rawIBFTValidatorType string
	ibftValidatorType    validators.ValidatorType

	// PoA
	ibftValidatorPrefixPath string
	ibftValidatorsRaw       []string
	ibftValidators          validators.Validators

	// PoS
	maxValidatorCountRaw string
	maxValidatorCount    *uint64
	minValidatorCountRaw string
	minValidatorCount    *uint64

	genesisConfig *chain.Chain
}

func (p *switchParams) getRequiredFlags() []string {
	return []string{
		typeFlag,
		fromFlag,
	}
}

func (p *switchParams) initRawParams() error {
	if err := p.initMechanismType(); err != nil {
		return err
	}

	if err := p.initIBFTValidatorType(); err != nil {
		return err
	}

	if err := p.initDeployment(); err != nil {
		return err
	}

	if err := p.initFrom(); err != nil {
		return err
	}

	if err := p.initPoAConfig(); err != nil {
		return err
	}

	if err := p.initPoSConfig(); err != nil {
		return err
	}

	if err := p.initChain(); err != nil {
		return err
	}

	return nil
}

func (p *switchParams) initMechanismType() error {
	ibftType, err := fork.ParseIBFTType(p.typeRaw)
	if err != nil {
		return fmt.Errorf("unable to parse mechanism type: %w", err)
	}

	p.ibftType = ibftType

	return nil
}

func (p *switchParams) initIBFTValidatorType() error {
	if p.rawIBFTValidatorType == "" {
		return nil
	}

	var err error

	if p.ibftValidatorType, err = validators.ParseValidatorType(p.rawIBFTValidatorType); err != nil {
		return err
	}

	return nil
}

func (p *switchParams) initDeployment() error {
	if p.deploymentRaw != "" {
		if p.ibftType != fork.PoS {
			return fmt.Errorf(
				"doesn't support contract deployment in %s",
				string(p.ibftType),
			)
		}

		d, err := types.ParseUint64orHex(&p.deploymentRaw)
		if err != nil {
			return fmt.Errorf(
				"unable to parse deployment value, %w",
				err,
			)
		}

		p.deployment = &d
	}

	return nil
}

func (p *switchParams) initPoAConfig() error {
	if p.ibftType != fork.PoA {
		return nil
	}

	p.ibftValidators = validators.NewValidatorSetFromType(p.ibftValidatorType)

	if err := p.setValidatorSetFromPrefixPath(); err != nil {
		return err
	}

	if err := p.setValidatorSetFromCli(); err != nil {
		return err
	}

	// Validate if validator number exceeds max number
	if uint64(p.ibftValidators.Len()) > common.MaxSafeJSInt {
		return command.ErrValidatorNumberExceedsMax
	}

	return nil
}

func (p *switchParams) setValidatorSetFromPrefixPath() error {
	if p.ibftValidatorPrefixPath == "" {
		return nil
	}

	validators, err := command.GetValidatorsFromPrefixPath(
		p.ibftValidatorPrefixPath,
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

// setValidatorSetFromCli sets validator set from cli command
func (p *switchParams) setValidatorSetFromCli() error {
	if len(p.ibftValidatorsRaw) == 0 {
		return nil
	}

	newSet, err := validators.ParseValidators(p.ibftValidatorType, p.ibftValidatorsRaw)
	if err != nil {
		return err
	}

	if err = p.ibftValidators.Merge(newSet); err != nil {
		return err
	}

	return nil
}

func (p *switchParams) initPoSConfig() error {
	if p.ibftType != fork.PoS {
		if p.minValidatorCountRaw != "" || p.maxValidatorCountRaw != "" {
			return fmt.Errorf(
				"doesn't support min validator count in %s",
				string(p.ibftType),
			)
		}

		return nil
	}

	if p.minValidatorCountRaw != "" {
		value, err := types.ParseUint64orHex(&p.minValidatorCountRaw)
		if err != nil {
			return fmt.Errorf(
				"unable to parse min validator count value, %w",
				err,
			)
		}

		p.minValidatorCount = &value
	}

	if p.maxValidatorCountRaw != "" {
		value, err := types.ParseUint64orHex(&p.maxValidatorCountRaw)
		if err != nil {
			return fmt.Errorf(
				"unable to parse max validator count value, %w",
				err,
			)
		}

		p.maxValidatorCount = &value
	}

	return p.validateMinMaxValidatorNumber()
}

func (p *switchParams) validateMinMaxValidatorNumber() error {
	// Validate min and max validators number if not nil
	// If they are not defined they will get default values
	// in PoSMechanism
	minValidatorCount := uint64(1)
	maxValidatorCount := common.MaxSafeJSInt

	if p.minValidatorCount != nil {
		minValidatorCount = *p.minValidatorCount
	}

	if p.maxValidatorCount != nil {
		maxValidatorCount = *p.maxValidatorCount
	}

	if err := command.ValidateMinMaxValidatorsNumber(minValidatorCount, maxValidatorCount); err != nil {
		return err
	}

	return nil
}

func (p *switchParams) initFrom() error {
	from, err := types.ParseUint64orHex(&p.fromRaw)
	if err != nil {
		return fmt.Errorf("unable to parse from value, %w", err)
	}

	if from <= 0 {
		return ErrFromPositive
	}

	p.from = from

	return nil
}

func (p *switchParams) initChain() error {
	cc, err := chain.Import(p.genesisPath)
	if err != nil {
		return fmt.Errorf(
			"failed to load chain config from %s: %w",
			p.genesisPath,
			err,
		)
	}

	p.genesisConfig = cc

	return nil
}

func (p *switchParams) updateGenesisConfig() error {
	return appendIBFTForks(
		p.genesisConfig,
		p.ibftType,
		p.ibftValidatorType,
		p.from,
		p.deployment,
		p.ibftValidators,
		p.maxValidatorCount,
		p.minValidatorCount,
	)
}

func (p *switchParams) overrideGenesisConfig() error {
	// Remove the current genesis configuration from disk
	if err := os.Remove(p.genesisPath); err != nil {
		return err
	}

	// Save the new genesis configuration
	if err := helper.WriteGenesisConfigToDisk(
		p.genesisConfig,
		p.genesisPath,
	); err != nil {
		return err
	}

	return nil
}

func (p *switchParams) getResult() command.CommandResult {
	result := &IBFTSwitchResult{
		Chain:         p.genesisPath,
		Type:          p.ibftType,
		ValidatorType: p.ibftValidatorType,
		From:          common.JSONNumber{Value: p.from},
	}

	if p.deployment != nil {
		result.Deployment = &common.JSONNumber{Value: *p.deployment}
	}

	if p.minValidatorCount != nil {
		result.MinValidatorCount = common.JSONNumber{Value: *p.minValidatorCount}
	} else {
		result.MinValidatorCount = common.JSONNumber{Value: 1}
	}

	if p.maxValidatorCount != nil {
		result.MaxValidatorCount = common.JSONNumber{Value: *p.maxValidatorCount}
	} else {
		result.MaxValidatorCount = common.JSONNumber{Value: common.MaxSafeJSInt}
	}

	return result
}

func appendIBFTForks(
	cc *chain.Chain,
	ibftType fork.IBFTType,
	validatorType validators.ValidatorType,
	from uint64,
	deployment *uint64,
	// PoA
	validators validators.Validators,
	// PoS
	maxValidatorCount *uint64,
	minValidatorCount *uint64,
) error {
	ibftConfig, ok := cc.Params.Engine["ibft"].(map[string]interface{})
	if !ok {
		return ErrIBFTConfigNotFound
	}

	ibftForks, err := fork.GetIBFTForks(ibftConfig)
	if err != nil {
		return err
	}

	lastFork := ibftForks[len(ibftForks)-1]

	if (ibftType == lastFork.Type) &&
		(validatorType == lastFork.ValidatorType) {
		return ErrSameIBFTAndValidatorType
	}

	if from <= lastFork.From.Value {
		return ErrLessFromThanLastFrom
	}

	if ibftType == fork.PoA && validators != nil && from <= 1 {
		// can't update validators at block 0
		return ErrInvalidValidatorsUpdateHeight
	}

	lastFork.To = &common.JSONNumber{Value: from - 1}

	newFork := fork.IBFTFork{
		Type:          ibftType,
		ValidatorType: validatorType,
		From:          common.JSONNumber{Value: from},
	}

	switch ibftType {
	case fork.PoA:
		newFork.Validators = validators
	case fork.PoS:
		if deployment != nil {
			newFork.Deployment = &common.JSONNumber{Value: *deployment}
		}

		if maxValidatorCount != nil {
			newFork.MaxValidatorCount = &common.JSONNumber{Value: *maxValidatorCount}
		}

		if minValidatorCount != nil {
			newFork.MinValidatorCount = &common.JSONNumber{Value: *minValidatorCount}
		}
	}

	ibftForks = append(ibftForks, &newFork)
	ibftConfig["types"] = ibftForks

	// remove leftover config
	delete(ibftConfig, "type")

	cc.Params.Engine["ibft"] = ibftConfig

	return nil
}
