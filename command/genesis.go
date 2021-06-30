package command

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/command/helper"
	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/0xPolygon/minimal/crypto"
	helperFlags "github.com/0xPolygon/minimal/helper/flags"
	"github.com/0xPolygon/minimal/types"
	"github.com/mitchellh/cli"
)

const (
	genesisFileName        = "./genesis.json"
	defaultChainName       = "example"
	defaultChainID         = 100
	defaultPremineBalance  = "0x3635C9ADC5DEA00000" // 1000 ETH
	defaultPrestakeBalance = "0x0"                  // 0 ETH
	defaultConsensus       = "pow"
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
		c.flagMap = make(map[string]helper.FlagDescriptor)
	}

	if len(c.flagMap) > 0 {
		// No need to redefine the flags again
		return
	}

	c.flagMap["data-dir"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the directory for the Polygon SDK data. Default: %s", genesisFileName),
		Arguments: []string{
			"DATA_DIRECTORY",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.flagMap["name"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the name for the chain. Default: %s", defaultChainName),
		Arguments: []string{
			"NAME",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.flagMap["premine"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the premined accounts and balances. Default premined balance: %s", defaultPremineBalance),
		Arguments: []string{
			"ADDRESS:VALUE",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.flagMap["prestake"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the prestaked balance for accounts. Default prestaked balance: %s", defaultPrestakeBalance),
		Arguments: []string{
			"ADDRESS:VALUE",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.flagMap["chainid"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the ID of the chain. Default: %d", defaultChainID),
		Arguments: []string{
			"CHAIN_ID",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.flagMap["consensus"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets consensus protocol. Default: %s", defaultConsensus),
		Arguments: []string{
			"CONSENSUS_PROTOCOL",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.flagMap["bootnode"] = helper.FlagDescriptor{
		Description: "Multiaddr URL for p2p discovery bootstrap. This flag can be used multiple times.",
		Arguments: []string{
			"BOOTNODE_URL",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.flagMap["ibft-validator"] = helper.FlagDescriptor{
		Description: "Sets passed in addresses as IBFT validators. Needs to be present if ibft-validators-prefix-path is omitted",
		Arguments: []string{
			"IBFT_VALIDATOR_LIST",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	c.flagMap["ibft-validators-prefix-path"] = helper.FlagDescriptor{
		Description: "Prefix path for validator folder directory. Needs to be present if ibft-validator is omitted",
		Arguments: []string{
			"IBFT_VALIDATORS_PREFIX_PATH",
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

	return helper.GenerateHelp(c.Synopsis(), helper.GenerateUsage(c.GetBaseCommand(), c.flagMap), c.flagMap)
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
	var prestake helperFlags.ArrayFlags
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
	flags.Var(&prestake, "prestake", "")
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

	for _, pres := range prestake {
		var addr types.Address
		val := defaultPrestakeBalance
		if indx := strings.Index(pres, ":"); indx != -1 {
			// <addr>:<balance>
			addr, val = types.StringToAddress(pres[:indx]), pres[indx+1:]
		} else {
			// <addr>
			addr = types.StringToAddress(pres)
		}

		stakeAmount, err := types.ParseUint256orHex(&val)
		if err != nil {
			c.UI.Error(fmt.Sprintf("failed to parse stake amount %s: %v", val, err))
			return 1
		}

		previousAccount := cc.Genesis.Alloc[addr]

		if previousAccount != nil {
			// Account already has a premined balance
			previousAccount.StakedBalance = stakeAmount
			cc.Genesis.Alloc[addr] = previousAccount
		} else {
			// Account doesn't have a premined balance
			cc.Genesis.Alloc[addr] = &chain.GenesisAccount{
				Balance:       big.NewInt(0),
				StakedBalance: stakeAmount,
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
