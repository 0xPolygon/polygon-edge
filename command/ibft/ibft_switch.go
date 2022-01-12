package ibft

import (
	"bytes"
	"encoding/json"
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
		From:  DecOrHexInt{from},
	}
	if deployment != nil {
		res.Deployment = &DecOrHexInt{*deployment}
	}

	c.Formatter.OutputResult(res)

	return 0
}

type IBFTSwitchResult struct {
	Chain      string             `json:"chain"`
	Type       ibft.MechanismType `json:"type"`
	From       DecOrHexInt        `json:"from"`
	Deployment *DecOrHexInt       `json:"deployment,omitempty"`
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

type DecOrHexInt struct {
	Value uint64
}

func (d *DecOrHexInt) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"0x%x"`, d.Value)), nil
}

func (d *DecOrHexInt) UnmarshalJSON(data []byte) error {
	var rawValue interface{}
	if err := json.Unmarshal(data, &rawValue); err != nil {
		return err
	}

	val, err := common.ConvertUnmarshalledInt(rawValue)
	if err != nil {
		return err
	}

	if val < 0 {
		return errors.New("must be positive value")
	}

	d.Value = uint64(val)

	return nil
}

type IBFTFork struct {
	Type       ibft.MechanismType `json:"type"`
	Deployment *DecOrHexInt       `json:"deployment,omitempty"`
	From       DecOrHexInt        `json:"from"`
	To         *DecOrHexInt       `json:"to,omitempty"`
}

// getIBFTForks returns IBFT fork configurations from chain config
func getIBFTForks(ibftConfig map[string]interface{}) ([]IBFTFork, error) {
	// no fork, only specifying IBFT type in chain config
	if originalType, ok := ibftConfig["type"].(string); ok {
		typ, err := ibft.ParseType(originalType)
		if err != nil {
			return nil, err
		}

		return []IBFTFork{
			{
				Type:       typ,
				Deployment: nil,
				From:       DecOrHexInt{0},
				To:         nil,
			},
		}, nil
	}

	// with forks
	if types, ok := ibftConfig["types"].([]interface{}); ok {
		bytes, err := json.Marshal(types)
		if err != nil {
			return nil, err
		}

		var forks []IBFTFork
		if err := json.Unmarshal(bytes, &forks); err != nil {
			return nil, err
		}

		return forks, nil
	}

	return nil, errors.New("current IBFT type not found")
}

func appendIBFTForks(cc *chain.Chain, typ ibft.MechanismType, from uint64, deployment *uint64) error {
	ibftConfig, ok := cc.Params.Engine["ibft"].(map[string]interface{})
	if !ok {
		return errors.New(`"ibft" setting doesn't exist in "engine" of genesis.json'`)
	}

	ibftForks, err := getIBFTForks(ibftConfig)
	if err != nil {
		return err
	}

	lastFork := &ibftForks[len(ibftForks)-1]
	if from <= lastFork.From.Value {
		return errors.New(`"from" must be greater than the beginning height of last fork`)
	}

	lastFork.To = &DecOrHexInt{from - 1}

	newFork := IBFTFork{
		Type: typ,
		From: DecOrHexInt{from},
	}
	if typ == ibft.PoS {
		newFork.Deployment = &DecOrHexInt{*deployment}
	}

	ibftForks = append(ibftForks, newFork)
	ibftConfig["types"] = ibftForks

	// remove leftover config
	delete(ibftConfig, "type")

	cc.Params.Engine["ibft"] = ibftConfig

	return nil
}
