package ibftswitch

import (
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/ibft"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"os"
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
	ErrFromPositive = errors.New(`"from" must be positive number`)
)

var (
	params = &switchParams{}
)

type switchParams struct {
	typeRaw              string
	fromRaw              string
	deploymentRaw        string
	maxValidatorCountRaw string
	minValidatorCountRaw string
	genesisPath          string

	mechanismType ibft.MechanismType
	deployment    *uint64
	from          uint64
	genesisConfig *chain.Chain

	maxValidatorCount *uint64
	minValidatorCount *uint64
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

	if err := p.initDeployment(); err != nil {
		return err
	}

	if err := p.initFrom(); err != nil {
		return err
	}

	if err := p.initChain(); err != nil {
		return err
	}

	return nil
}

func (p *switchParams) initMechanismType() error {
	mechanismType, err := ibft.ParseType(p.typeRaw)
	if err != nil {
		return fmt.Errorf("unable to parse mechanism type: %w", err)
	}

	p.mechanismType = mechanismType

	return nil
}

func (p *switchParams) initDeployment() error {
	if p.deploymentRaw != "" {
		if p.mechanismType != ibft.PoS {
			return fmt.Errorf(
				"doesn't support contract deployment in %s",
				string(p.mechanismType),
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

	if p.minValidatorCountRaw != "" {
		if p.mechanismType != ibft.PoS {
			return fmt.Errorf(
				"doesn't support min validator count in %s",
				string(p.mechanismType),
			)
		}

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
		if p.mechanismType != ibft.PoS {
			return fmt.Errorf(
				"doesn't support max validator count in %s",
				string(p.mechanismType),
			)
		}

		value, err := types.ParseUint64orHex(&p.maxValidatorCountRaw)
		if err != nil {
			return fmt.Errorf(
				"unable to parse max validator count value, %w",
				err,
			)
		}

		p.maxValidatorCount = &value
	}

	if err := p.validateMinMaxValidatorNumber(); err != nil {
		return err
	}

	return nil
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
		p.mechanismType,
		p.from,
		p.deployment,
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
		Chain: p.genesisPath,
		Type:  p.mechanismType,
		From:  common.JSONNumber{Value: p.from},
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
	mechanismType ibft.MechanismType,
	from uint64,
	deployment *uint64,
	maxValidatorCount *uint64,
	minValidatorCount *uint64,
) error {
	ibftConfig, ok := cc.Params.Engine["ibft"].(map[string]interface{})
	if !ok {
		return errors.New(`"ibft" setting doesn't exist in "engine" of genesis.json'`)
	}

	ibftForks, err := ibft.GetIBFTForks(ibftConfig)
	if err != nil {
		return err
	}

	lastFork := &ibftForks[len(ibftForks)-1]
	if mechanismType == lastFork.Type {
		return errors.New(`cannot specify same IBFT type to the last fork`)
	}

	if from <= lastFork.From.Value {
		return errors.New(`"from" must be greater than the beginning height of last fork`)
	}

	lastFork.To = &common.JSONNumber{Value: from - 1}

	newFork := ibft.IBFTFork{
		Type: mechanismType,
		From: common.JSONNumber{Value: from},
	}

	if mechanismType == ibft.PoS {
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

	ibftForks = append(ibftForks, newFork)
	ibftConfig["types"] = ibftForks

	// remove leftover config
	delete(ibftConfig, "type")

	cc.Params.Engine["ibft"] = ibftConfig

	return nil
}
