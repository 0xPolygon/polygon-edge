package command

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xPolygon/minimal/chain"
	helperFlags "github.com/0xPolygon/minimal/helper/flags"
	"github.com/0xPolygon/minimal/types"
	"github.com/mitchellh/cli"
)

const (
	genesisFileName       = "./genesis.json"
	defaultChainID        = 100
	defaultPremineBalance = "0x100000000000000000000000000"
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

	flags := flag.NewFlagSet("genesis", flag.ContinueOnError)
	flags.Usage = func() {}

	var dataDir string
	var premine helperFlags.ArrayFlags
	var chainID uint64

	flags.StringVar(&dataDir, "data-dir", "", "")
	flags.Var(&premine, "premine", "")
	flags.Uint64Var(&chainID, "chainid", 100, "")

	if err := flags.Parse(args); err != nil {
		c.UI.Error(fmt.Sprintf("failed to parse args: %v", err))
		return 1
	}

	genesisPath := filepath.Join(dataDir, genesisFileName)
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
			Alloc:      map[types.Address]*chain.GenesisAccount{},
		},
		Params: &chain.Params{
			ChainID: int(chainID),
			Forks:   &chain.Forks{},
			Engine: map[string]interface{}{
				"pow": map[string]interface{}{},
			},
		},
		Bootnodes: chain.Bootnodes{},
	}

	if len(premine) != 0 {
		for _, prem := range premine {

			var addr types.Address
			val := defaultPremineBalance
			if indx := strings.Index(prem, ":"); indx != -1 {
				// <addr>:<balance>
				addr, val = types.StringToAddress(prem[:indx]), prem[indx+1:]
			} else {
				// <addr>
				addr = types.StringToAddress(prem)
			}

			amount, err := types.ParseUint256orHex(&val)
			if err != nil {
				c.UI.Error(fmt.Sprintf("failed to parse amount %s: %v", val, err))
				return 1
			}
			cc.Genesis.Alloc[addr] = &chain.GenesisAccount{
				Balance: amount,
			}
		}
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
