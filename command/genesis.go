package command

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/0xPolygon/minimal/helper/flags"
	"github.com/0xPolygon/minimal/types"
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
	var ibftConsensus bool
	var chainID uint64
	var name string
	var gasLimit uint64
	var validators flags.ArrayFlags

	flags := flag.NewFlagSet("genesis", flag.ContinueOnError)
	flags.BoolVar(&ibftConsensus, "ibft", false, "")
	flags.Uint64Var(&chainID, "chainid", 100, "")
	flags.StringVar(&name, "name", "example", "")
	flags.Uint64Var(&gasLimit, "gasLimit", 5000, "")
	flags.Var(&validators, "validators", "Ibft validators")

	if err := flags.Parse(args); err != nil {
		c.UI.Error(fmt.Sprintf("Failed to parse args: %v", err))
		return 1
	}

	_, err := os.Stat(genesisPath)
	if err != nil && !os.IsNotExist(err) {
		c.UI.Error(fmt.Sprintf("Failed to stat (%s): %v", genesisPath, err))
		return 1
	}
	if !os.IsNotExist(err) {
		c.UI.Error(fmt.Sprintf("Genesis (%s) already exists", genesisPath))
		return 1
	}

	consensus := "pow"
	var extraData []byte

	if ibftConsensus {
		consensus = "ibft"

		if len(validators) == 0 {
			c.UI.Error("No validators found for ibft consensus")
			return 1
		}

		// create the validators list for the extra data
		validatorsAddrs := []types.Address{}
		for _, val := range validators {
			key := types.StringToAddress(val)
			validatorsAddrs = append(validatorsAddrs, key)
		}

		ibftExtra := &ibft.IstanbulExtra{
			Validators:    validatorsAddrs,
			Seal:          []byte{},
			CommittedSeal: [][]byte{},
		}

		extraData = make([]byte, types.IstanbulExtraVanity)
		extraData = ibftExtra.MarshalRLPTo(extraData)
	}

	cc := &chain.Chain{
		Name: name,
		Genesis: &chain.Genesis{
			GasLimit:   gasLimit,
			Difficulty: 1,
			ExtraData:  extraData,
		},
		Params: &chain.Params{
			ChainID: int(chainID),
			Forks: &chain.Forks{
				Homestead: chain.NewFork(0),
				EIP150:    chain.NewFork(0),
				EIP155:    chain.NewFork(0),
				EIP158:    chain.NewFork(0),
			},
			Engine: map[string]interface{}{
				consensus: map[string]interface{}{},
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
