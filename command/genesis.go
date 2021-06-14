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
	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/0xPolygon/minimal/crypto"
	helperFlags "github.com/0xPolygon/minimal/helper/flags"
	"github.com/0xPolygon/minimal/types"
	"github.com/mitchellh/cli"
)

const (
	genesisFileName       = "./genesis.json"
	defaultChainName      = "example"
	defaultChainID        = 100
	defaultPremineBalance = "0x3635C9ADC5DEA00000" // 1000 ETH
	defaultConsensus      = "pow"
)

// GenesisCommand is the command to show the version of the agent
type GenesisCommand struct {
	UI cli.Ui
	Meta
}

// DefineFlags defines the command flags
func (c *GenesisCommand) DefineFlags() {
	if c.flagMap == nil {
		// Flag map not initialized
		c.flagMap = make(map[string]types.FlagDescriptor)
	}

	if len(c.flagMap) > 0 {
		// No need to redefine the flags again
		return
	}

	c.flagMap["data-dir"] = MetaFlagDescriptor{
		description: fmt.Sprintf("Sets the directory for the Polygon SDK data. Default: %s", genesisFileName),
		arguments: []string{
			"DATA_DIRECTORY",
		},
		argumentsOptional: false,
		flagOptional:      true,
	}

	c.flagMap["name"] = MetaFlagDescriptor{
		description: fmt.Sprintf("Sets the name for the chain. Default: %s", defaultChainName),
		arguments: []string{
			"NAME",
		},
		argumentsOptional: false,
		flagOptional:      true,
	}

	c.flagMap["premine"] = MetaFlagDescriptor{
		description: fmt.Sprintf("Sets the premined accounts and balances. Default premined balance: %s", defaultPremineBalance),
		arguments: []string{
			"ADDRESS:VALUE",
		},
		argumentsOptional: false,
		flagOptional:      true,
	}

	c.flagMap["chainid"] = MetaFlagDescriptor{
		description: fmt.Sprintf("Sets the ID of the chain. Default: %d", defaultChainID),
		arguments: []string{
			"CHAIN_ID",
		},
		argumentsOptional: false,
		flagOptional:      true,
	}

	c.flagMap["consensus"] = MetaFlagDescriptor{
		description: fmt.Sprintf("Sets consensus protocol. Default: %s", defaultConsensus),
		arguments: []string{
			"CONSENSUS_PROTOCOL",
		},
		argumentsOptional: false,
		flagOptional:      true,
	}

	c.flagMap["bootnode"] = MetaFlagDescriptor{
		description: "Multiaddr URL for p2p discovery bootstrap. This flag can be used multiple times.",
		arguments: []string{
			"BOOTNODE_URL",
		},
		argumentsOptional: false,
		flagOptional:      true,
	}

	c.flagMap["ibft-validator"] = MetaFlagDescriptor{
		description: "Sets passed in addresses as IBFT validators. Needs to be present if ibft-validators-prefix-path is omitted",
		arguments: []string{
			"IBFT_VALIDATOR_LIST",
		},
		argumentsOptional: false,
		flagOptional:      true,
	}

	c.flagMap["ibft-validators-prefix-path"] = MetaFlagDescriptor{
		description: "Prefix path for validator folder directory. Needs to be present if ibft-validator is omitted",
		arguments: []string{
			"IBFT_VALIDATORS_PREFIX_PATH",
		},
		argumentsOptional: false,
		flagOptional:      true,
	}
}

// GetHelperText returns a simple description of the command
func (c *GenesisCommand) GetHelperText() string {
	return "Generates the genesis.json file, with passed in parameters"
}

func (c *GenesisCommand) GetBaseCommand() string {
	return "genesis"
}

// Help implements the cli.Command interface
func (c *GenesisCommand) Help() string {
	c.DefineFlags()

	return types.GenerateHelp(c.Synopsis(), types.GenerateUsage(c.GetBaseCommand(), c.flagMap), c.flagMap)
}

// Synopsis implements the cli.Command interface
func (c *GenesisCommand) Synopsis() string {
	return c.GetHelperText()
}

// Run implements the cli.Command interface
func (c *GenesisCommand) Run(args []string) int {
	flags := flag.NewFlagSet(c.GetBaseCommand(), flag.ContinueOnError)
	flags.Usage = func() {}

	var dataDir string
	var premine helperFlags.ArrayFlags
	var chainID uint64
	var bootnodes = make(helperFlags.BootnodeFlags, 0)
	var name string
	var consensus string

	// ibft flags
	var ibftValidators helperFlags.ArrayFlags
	var ibftValidatorsPrefixPath string

	flags.StringVar(&dataDir, "data-dir", "", "")
	flags.StringVar(&name, "name", defaultChainName, "")
	flags.Var(&premine, "premine", "")
	flags.Uint64Var(&chainID, "chainid", defaultChainID, "")
	flags.Var(&bootnodes, "bootnode", "")
	flags.StringVar(&consensus, "consensus", defaultConsensus, "")
	flags.Var(&ibftValidators, "ibft-validator", "list of ibft validators")
	flags.StringVar(&ibftValidatorsPrefixPath, "ibft-validators-prefix-path", "", "")

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

	var extraData []byte

	if consensus == "ibft" {
		// we either use validatorsFlags or ibftValidatorsPrefixPath to set the validators
		var validators []types.Address
		if len(ibftValidators) != 0 {
			for _, val := range ibftValidators {
				validators = append(validators, types.StringToAddress(val))
			}
		} else if ibftValidatorsPrefixPath != "" {
			// read all folders with the ibftValidatorsPrefixPath and search for Istanbul addresses
			if validators, err = readValidatorsByRegexp(ibftValidatorsPrefixPath); err != nil {
				c.UI.Error(fmt.Sprintf("failed to read from prefix: %v", err))
				return 1
			}

		} else {
			c.UI.Error("cannot load validators for ibft")
			return 1
		}

		// create the initial extra data with the validators
		ibftExtra := &ibft.IstanbulExtra{
			Validators:    validators,
			Seal:          []byte{},
			CommittedSeal: [][]byte{},
		}
		extraData = make([]byte, ibft.IstanbulExtraVanity)
		extraData = ibftExtra.MarshalRLPTo(extraData)
	}

	cc := &chain.Chain{
		Name: name,
		Genesis: &chain.Genesis{
			GasLimit:   5000,
			Difficulty: 1,
			Alloc:      map[types.Address]*chain.GenesisAccount{},
			ExtraData:  extraData,
		},
		Params: &chain.Params{
			ChainID: int(chainID),
			Forks:   chain.AllForksEnabled,
			Engine: map[string]interface{}{
				consensus: map[string]interface{}{},
			},
		},
		Bootnodes: bootnodes,
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

func readValidatorsByRegexp(prefix string) ([]types.Address, error) {
	validators := []types.Address{}

	files, err := ioutil.ReadDir(".")
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		path := file.Name()
		if !file.IsDir() {
			continue
		}
		if !strings.HasPrefix(path, prefix) {
			continue
		}

		// try to read key from the filepath/consensus/<key> path
		possibleConsensusPath := filepath.Join(path, "consensus", ibft.IbftKeyName)

		// check if path exists
		if _, err := os.Stat(possibleConsensusPath); os.IsNotExist(err) {
			continue
		}

		priv, err := crypto.ReadPrivKey(possibleConsensusPath)
		if err != nil {
			return nil, err
		}
		validators = append(validators, crypto.PubKeyToAddress(&priv.PublicKey))
	}

	return validators, nil
}
