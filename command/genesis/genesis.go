package command

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/spf13/cobra"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/command"
)

const genesisPath = "./genesis.json"
const defaultChainID = 100

var genesisCmd = &cobra.Command{
	Use:   "genesis",    // TODO: change to a compiler input string?
	Short: "Genesis...", // TODO
	Run:   genesisRun,
	RunE:  genesisRunE,
}

func init() {
	command.RegisterCmd(genesisCmd)
}

func genesisRun(cmd *cobra.Command, args []string) {
	command.RunCmd(cmd, args, genesisRunE)
}

func genesisRunE(cmd *cobra.Command, args []string) (err error) {
	_, err = os.Stat(genesisPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Failed to stat (%s): %v", genesisPath, err)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("Genesis (%s) already exists", genesisPath)
	}

	c := &chain.Chain{
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

	data, err := json.MarshalIndent(c, "", "    ")
	if err != nil {
		err = fmt.Errorf("Failed to generate genesis: %v", err)
	}

	if err == nil {
		if err = ioutil.WriteFile(genesisPath, data, 0644); err != nil {
			err = fmt.Errorf("Failed to write genesis: %v", err)
		}
	}

	if err == nil {
		fmt.Printf("Genesis written to %s\n", genesisPath)
	}

	return err
}
