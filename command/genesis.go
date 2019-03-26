package command

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"

	"github.com/umbracle/minimal/chain"
)

const genesisPath = "./genesis.json"
const defaultChainID = 100

type GenesisCommand struct {
	Meta
}

func (g *GenesisCommand) Help() string {
	return ""
}

func (g *GenesisCommand) Synopsis() string {
	return ""
}

func (g *GenesisCommand) Run(args []string) int {

	_, err := os.Stat(genesisPath)
	if err != nil && !os.IsNotExist(err) {
		g.Ui.Error(fmt.Sprintf("Failed to stat (%s): %v", genesisPath, err))
		return 1
	}
	if !os.IsNotExist(err) {
		g.Ui.Error(fmt.Sprintf("Genesis (%s) already exists", genesisPath))
		return 1
	}

	c := &chain.Chain{
		Name: "example",
		Genesis: &chain.Genesis{
			Nonce:      1,
			GasLimit:   5000,
			Difficulty: big.NewInt(1),
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

	data, err := json.MarshalIndent(c, "", "    ")
	if err != nil {
		g.Ui.Error(fmt.Sprintf("Failed to generate genesis: %v", err))
		return 1
	}

	if err := ioutil.WriteFile(genesisPath, data, 0644); err != nil {
		g.Ui.Error(fmt.Sprintf("Failed to write genesis: %v", err))
		return 1
	}

	g.Ui.Output(fmt.Sprintf("Genesis written to %s", genesisPath))
	return 0
}
