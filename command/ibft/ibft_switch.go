package ibft

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/consensus/ibft"
	"github.com/0xPolygon/polygon-sdk/helper/common"
	"github.com/0xPolygon/polygon-sdk/types"
)

// IBFTSwitchCommand is the command to switch consensus
type IBFTSwitchCommand struct {
	helper.Base
	Formatter *helper.FormatterFlag
}

func (c *IBFTSwitchCommand) DefineFlags() {
	c.Base.DefineFlags(c.Formatter)

	c.FlagMap["chain"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Specifies the genesis file to update. Default: %s", helper.DefaultConfig().Chain),
		Arguments: []string{
			"GENESIS_FILE",
		},
		FlagOptional: true,
	}

	c.FlagMap["type"] = helper.FlagDescriptor{
		Description: "Sets the new IBFT type [PoA, PoS]",
		Arguments: []string{
			"TYPE",
		},
		FlagOptional:      false,
		ArgumentsOptional: false,
	}

	c.FlagMap["deployment"] = helper.FlagDescriptor{
		Description: "Sets the height to deploy the contract in PoS",
		Arguments: []string{
			"DEPLOYMENT",
		},
		FlagOptional:      true,
		ArgumentsOptional: false,
	}

	c.FlagMap["from"] = helper.FlagDescriptor{
		Description: "Sets the height to switch the new type",
		Arguments: []string{
			"FROM",
		},
		FlagOptional:      false,
		ArgumentsOptional: false,
	}
}

// GetHelperText returns a simple description of the command
func (c *IBFTSwitchCommand) GetHelperText() string {
	return "Add settings in genesis.json to switch IBFT type"
}

func (c *IBFTSwitchCommand) GetBaseCommand() string {
	return "ibft switch"
}

// Help implements the cli.PeersAdd interface
func (c *IBFTSwitchCommand) Help() string {
	c.DefineFlags()

	return helper.GenerateHelp(c.Synopsis(), helper.GenerateUsage(c.GetBaseCommand(), c.FlagMap), c.FlagMap)
}

// Synopsis implements the cli.PeersAdd interface
func (c *IBFTSwitchCommand) Synopsis() string {
	return c.GetHelperText()
}

// Run implements the cli.PeersAdd interface
func (c *IBFTSwitchCommand) Run(args []string) int {
	flags := c.Base.NewFlagSet(c.GetBaseCommand(), c.Formatter)

	var genesisPath, rawType, rawDeployment, rawFrom string

	flags.StringVar(&genesisPath, "chain", helper.DefaultConfig().Chain, "")
	flags.StringVar(&rawType, "type", "", "")
	flags.StringVar(&rawDeployment, "deployment", "", "")
	flags.StringVar(&rawFrom, "from", "", "")

	if err := flags.Parse(args); err != nil {
		c.Formatter.OutputError(err)

		return 1
	}

	typ, err := ibft.ParseType(rawType)
	if err != nil {
		c.Formatter.OutputError(err)

		return 1
	}

	var deployment *uint64

	if rawDeployment != "" {
		if typ == ibft.PoS {
			d, err := types.ParseUint64orHex(&rawDeployment)
			if err != nil {
				c.Formatter.OutputError(err)

				return 1
			}

			deployment = &d
		} else {
			c.Formatter.OutputError(fmt.Errorf(fmt.Sprintf("doesn't support contract deployment in %s", string(typ))))

			return 1
		}
	}

	from, err := types.ParseUint64orHex(&rawFrom)
	if err != nil {
		c.Formatter.OutputError(err)

		return 1
	}

	if from <= 0 {
		c.Formatter.OutputError(errors.New(`"from" must be positive number`))

		return 1
	}

	cc, err := chain.Import(genesisPath)
	if err != nil {
		c.Formatter.OutputError(fmt.Errorf("failed to load chain config from %s: %w", genesisPath, err))

		return 1
	}

	if err := appendIBFTForks(cc, typ, from, deployment); err != nil {
		c.Formatter.OutputError(err)

		return 1
	}

	if err := os.Remove(genesisPath); err != nil {
		c.Formatter.OutputError(err)

		return 1
	}

	if err = helper.WriteGenesisToDisk(cc, genesisPath); err != nil {
		c.UI.Error(err.Error())

		return 1
	}

	res := &IBFTSwitchResult{
		Chain: genesisPath,
		Type:  typ,
		From:  common.JSONNumber{Value: from},
	}
	if deployment != nil {
		res.Deployment = &common.JSONNumber{Value: *deployment}
	}

	c.Formatter.OutputResult(res)

	return 0
}

type IBFTSwitchResult struct {
	Chain      string             `json:"chain"`
	Type       ibft.MechanismType `json:"type"`
	From       common.JSONNumber  `json:"from"`
	Deployment *common.JSONNumber `json:"deployment,omitempty"`
}

func (r *IBFTSwitchResult) Output() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[MEW IBFT FORK]\n")

	outputs := []string{
		fmt.Sprintf("Chain|%s", r.Chain),
		fmt.Sprintf("Type|%s", r.Type),
	}
	if r.Deployment != nil {
		outputs = append(outputs, fmt.Sprintf("Deployment|%d", r.Deployment.Value))
	}

	outputs = append(outputs, fmt.Sprintf("Deployment|%d", r.From.Value))

	buffer.WriteString(helper.FormatKV(outputs))
	buffer.WriteString("\n")

	return buffer.String()
}

func appendIBFTForks(cc *chain.Chain, typ ibft.MechanismType, from uint64, deployment *uint64) error {
	ibftConfig, ok := cc.Params.Engine["ibft"].(map[string]interface{})
	if !ok {
		return errors.New(`"ibft" setting doesn't exist in "engine" of genesis.json'`)
	}

	ibftForks, err := ibft.GetIBFTForks(ibftConfig)
	if err != nil {
		return err
	}

	lastFork := &ibftForks[len(ibftForks)-1]
	if typ == lastFork.Type {
		return errors.New(`cannot specify same IBFT type to the last fork`)
	}
	if from <= lastFork.From.Value {
		return errors.New(`"from" must be greater than the beginning height of last fork`)
	}

	lastFork.To = &common.JSONNumber{Value: from - 1}

	newFork := ibft.IBFTFork{
		Type: typ,
		From: common.JSONNumber{Value: from},
	}
	if typ == ibft.PoS {
		newFork.Deployment = &common.JSONNumber{Value: *deployment}
	}

	ibftForks = append(ibftForks, newFork)
	ibftConfig["types"] = ibftForks

	// remove leftover config
	delete(ibftConfig, "type")

	cc.Params.Engine["ibft"] = ibftConfig

	return nil
}
