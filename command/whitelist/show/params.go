package show

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
)

const (
	chainFlag = "chain"
)

var (
	params = &showParams{}
)

type showParams struct {
	// genesis file
	genesisPath   string
	genesisConfig *chain.Chain
}

func (p *showParams) initRawParams() error {
	// init genesis configuration
	if err := p.initChain(); err != nil {
		return err
	}

	return nil
}

func (p *showParams) initChain() error {
	// import genesis configuration
	cc, err := chain.Import(p.genesisPath)
	if err != nil {
		return fmt.Errorf(
			"failed to load chain config from %s: %w",
			p.genesisPath,
			err,
		)
	}

	// set genesis configuration
	p.genesisConfig = cc

	return nil
}

func (p *showParams) getResult() command.CommandResult {
	result := &ShowResult{
		GenesisConfig: p.genesisConfig,
	}

	return result
}
