package command

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/0xPolygon/minimal/chain"
	"github.com/mitchellh/cli"
)

const (
	genesisPath    = "./genesis.json"
	defaultChainID = 100
)

// GenesisCommand is the command to show the version of the agent
type GenesisCommand struct {
	UI cli.Ui
}

// Help implements the cli.Command interface
func (c *GenesisCommand) Help() string {
	return ""
}

// Synopsis implements the cli.Command interface
func (c *GenesisCommand) Synopsis() string {
	return ""
}

// Run implements the cli.Command interface
func (c *GenesisCommand) Run(args []string) int {
	_, err := os.Stat(genesisPath)
	if err != nil && !os.IsNotExist(err) {
		c.UI.Error(fmt.Sprintf("Failed to stat (%s): %v", genesisPath, err))
		return 1
	}
	if !os.IsNotExist(err) {
		c.UI.Error(fmt.Sprintf("Genesis (%s) already exists", genesisPath))
		return 1
	}

	cc := &chain.Chain{
		Name: "example",
		Genesis: &chain.Genesis{
			GasLimit:   5000,
			Difficulty: 1,
		},
		Params: &chain.Params{
			ChainID: 100,
			Forks:   &chain.Forks{},
			Engine: map[string]interface{}{
				"pow": map[string]interface{}{},
			},
		},
		Bootnodes: chain.Bootnodes{},
	}

	data, err := json.MarshalIndent(cc, "", "    ")
	if err != nil {
		c.UI.Error(fmt.Sprintf("Failed to generate genesis: %v", err))
		return 1
	}
	if err := ioutil.WriteFile(genesisPath, data, 0644); err != nil {
		c.UI.Error(fmt.Sprintf("Failed to write genesis: %v", err))
		return 1
	}

	c.UI.Info(fmt.Sprintf("Genesis written to %s", genesisPath))
	return 0
}
