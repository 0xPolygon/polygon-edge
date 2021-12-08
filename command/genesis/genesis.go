package genesis

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/consensus/ibft"
	"github.com/0xPolygon/polygon-sdk/crypto"
	helperFlags "github.com/0xPolygon/polygon-sdk/helper/flags"
	"github.com/0xPolygon/polygon-sdk/types"
)

// GenesisCommand is the command to show the version of the agent
type GenesisCommand struct {
	helper.Base
}

// DefineFlags defines the command flags
func (c *GenesisCommand) DefineFlags() {
	c.Base.DefineFlags()

	if len(c.FlagMap) > 0 {
		// No need to redefine the flags again
		return
	}

	c.FlagMap["dir"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the directory for the Polygon SDK genesis data. Default: %s", helper.GenesisFileName),
		Arguments: []string{
			"DIRECTORY",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.FlagMap["name"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the name for the chain. Default: %s", helper.DefaultChainName),
		Arguments: []string{
			"NAME",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.FlagMap["premine"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the premined accounts and balances. Default premined balance: %s", helper.DefaultPremineBalance),
		Arguments: []string{
			"ADDRESS:VALUE",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.FlagMap["chainid"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the ID of the chain. Default: %d", helper.DefaultChainID),
		Arguments: []string{
			"CHAIN_ID",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.FlagMap["consensus"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets consensus protocol. Default: %s", helper.DefaultConsensus),
		Arguments: []string{
			"CONSENSUS_PROTOCOL",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.FlagMap["bootnode"] = helper.FlagDescriptor{
		Description: "Multiaddr URL for p2p discovery bootstrap. This flag can be used multiple times.",
		Arguments: []string{
			"BOOTNODE_URL",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.FlagMap["ibft-validator"] = helper.FlagDescriptor{
		Description: "Sets passed in addresses as IBFT validators. Needs to be present if ibft-validators-prefix-path is omitted",
		Arguments: []string{
			"IBFT_VALIDATOR_LIST",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.FlagMap["ibft-validators-prefix-path"] = helper.FlagDescriptor{
		Description: "Prefix path for validator folder directory. Needs to be present if ibft-validator is omitted",
		Arguments: []string{
			"IBFT_VALIDATORS_PREFIX_PATH",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.FlagMap["block-gas-limit"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Refers to the maximum amount of gas used by all operations in a block. Default: %d", helper.GenesisGasLimit),
		Arguments: []string{
			"BLOCK_GAS_LIMIT",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
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

	return helper.GenerateHelp(c.Synopsis(), helper.GenerateUsage(c.GetBaseCommand(), c.FlagMap), c.FlagMap)
}

// Synopsis implements the cli.Command interface
func (c *GenesisCommand) Synopsis() string {
	return c.GetHelperText()
}

// Run implements the cli.Command interface
func (c *GenesisCommand) Run(args []string) int {
	flags := c.NewFlagSet(c.GetBaseCommand())
	flags.Usage = func() {}

	var baseDir string
	var premine helperFlags.ArrayFlags
	var chainID uint64
	var bootnodes = helperFlags.BootnodeFlags{AreSet: false, Addrs: make([]string, 0)}
	var name string
	var consensus string

	// ibft flags
	var ibftValidators helperFlags.ArrayFlags
	var ibftValidatorsPrefixPath string

	var blockGasLimit uint64

	flags.StringVar(&baseDir, "dir", "", "")
	flags.StringVar(&name, "name", helper.DefaultChainName, "")
	flags.Var(&premine, "premine", "")
	flags.Uint64Var(&chainID, "chainid", helper.DefaultChainID, "")
	flags.Var(&bootnodes, "bootnode", "")
	flags.StringVar(&consensus, "consensus", helper.DefaultConsensus, "")
	flags.Var(&ibftValidators, "ibft-validator", "list of ibft validators")
	flags.StringVar(&ibftValidatorsPrefixPath, "ibft-validators-prefix-path", "", "")
	flags.Uint64Var(&blockGasLimit, "block-gas-limit", helper.GenesisGasLimit, "")

	if err := flags.Parse(args); err != nil {
		c.UI.Error(fmt.Sprintf("failed to parse args: %v", err))
		return 1
	}
	var err error = nil

	genesisPath := filepath.Join(baseDir, helper.GenesisFileName)
	if generateError := helper.VerifyGenesisExistence(genesisPath); generateError != nil {
		c.UI.Error(generateError.GetMessage())
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

	if bootnodes.AreSet && len(bootnodes.Addrs) < 2 {
		c.UI.Error("Minimum two bootnodes are required")
		return 1
	}

	cc := &chain.Chain{
		Name: name,
		Genesis: &chain.Genesis{
			GasLimit:   blockGasLimit,
			Difficulty: 1,
			Alloc:      map[types.Address]*chain.GenesisAccount{},
			ExtraData:  extraData,
			GasUsed:    helper.GenesisGasUsed,
		},
		Params: &chain.Params{
			ChainID: int(chainID),
			Forks:   chain.AllForksEnabled,
			Engine: map[string]interface{}{
				consensus: map[string]interface{}{},
			},
		},
		Bootnodes: bootnodes.Addrs,
	}

	if err = helper.FillPremineMap(cc.Genesis.Alloc, premine); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if err = helper.WriteGenesisToDisk(cc, genesisPath); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	output := "\n[GENESIS SUCCESS]\n"
	output += fmt.Sprintf("Genesis written to %s\n", genesisPath)

	c.UI.Info(output)

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

		priv, err := crypto.GenerateOrReadPrivateKey(possibleConsensusPath)
		if err != nil {
			return nil, err
		}
		validators = append(validators, crypto.PubKeyToAddress(&priv.PublicKey))
	}

	return validators, nil
}
