package quorum

import (
	"errors"
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/helper/common"
)

const (
	fromFlag  = "from"
	chainFlag = "chain"
)

var (
	params = &quorumParams{}
)

type quorumParams struct {
	genesisConfig *chain.Chain
	from          uint64
	genesisPath   string
}

func (p *quorumParams) initChain() error {
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

func (p *quorumParams) initRawParams() error {
	return p.initChain()
}

func (p *quorumParams) getRequiredFlags() []string {
	return []string{
		fromFlag,
	}
}

func (p *quorumParams) updateGenesisConfig() error {
	return appendIBFTQuorum(
		p.genesisConfig,
		p.from,
	)
}

func (p *quorumParams) overrideGenesisConfig() error {
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

func (p *quorumParams) getResult() command.CommandResult {
	return &IBFTQuorumResult{
		Chain: p.genesisPath,
		From:  common.JSONNumber{Value: p.from},
	}
}

func appendIBFTQuorum(
	cc *chain.Chain,
	from uint64,
) error {
	ibftConfig, ok := cc.Params.Engine["ibft"].(map[string]interface{})
	if !ok {
		return errors.New(`"ibft" setting doesn't exist in "engine" of genesis.json'`)
	}

	ibftConfig["quorumSizeBlockNum"] = from

	cc.Params.Engine["ibft"] = ibftConfig

	return nil
}
