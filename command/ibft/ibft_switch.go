package ibft

import (
	"fmt"
	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/consensus/ibft"
	"github.com/0xPolygon/polygon-sdk/types"
	"os"
)

// IBFTSwitchCommand is the command to switch consensus
type IBFTSwitchCommand struct {
	helper.Base
}

func (i *IBFTSwitchCommand) DefineFlags() {
	i.Base.DefineFlags()

	i.FlagMap["chain"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Specifies the genesis file to update. Default: %s", helper.DefaultConfig().Chain),
		Arguments: []string{
			"GENESIS_FILE",
		},
		FlagOptional: true,
	}

	i.FlagMap["type"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the new consensus type [PoA, PoS]"),
		Arguments: []string{
			"TYPE",
		},
		FlagOptional:      false,
		ArgumentsOptional: false,
	}

	i.FlagMap["deployment"] = helper.FlagDescriptor{
		Description: "Sets the height to deploy the contract",
		Arguments: []string{
			"DEPLOYMENT",
		},
		FlagOptional:      true,
		ArgumentsOptional: false,
	}

	i.FlagMap["from"] = helper.FlagDescriptor{
		Description: "Sets the height to start new consensus",
		Arguments: []string{
			"FROM",
		},
		FlagOptional:      true,
		ArgumentsOptional: false,
	}
}

// GetHelperText returns a simple description of the command
func (i *IBFTSwitchCommand) GetHelperText() string {
	return "Add new settings in genesis.json to switch IBFT mode"
}

func (i *IBFTSwitchCommand) GetBaseCommand() string {
	return "ibft switch"
}

// Help implements the cli.PeersAdd interface
func (i *IBFTSwitchCommand) Help() string {
	i.DefineFlags()

	return helper.GenerateHelp(i.Synopsis(), helper.GenerateUsage(i.GetBaseCommand(), i.FlagMap), i.FlagMap)
}

// Synopsis implements the cli.PeersAdd interface
func (i *IBFTSwitchCommand) Synopsis() string {
	return i.GetHelperText()
}

// Run implements the cli.PeersAdd interface
func (i *IBFTSwitchCommand) Run(args []string) int {
	flags := i.Base.NewFlagSet(i.GetBaseCommand())

	var genesisPath, rawType, rawDeployment, rawFrom string
	flags.StringVar(&genesisPath, "chain", helper.DefaultConfig().Chain, "")
	flags.StringVar(&rawType, "type", "", "")
	flags.StringVar(&rawDeployment, "deployment", "", "")
	flags.StringVar(&rawFrom, "from", "", "")

	if err := flags.Parse(args); err != nil {
		i.UI.Error(err.Error())
		return 1
	}

	typ, err := ibft.ParseType(rawType)
	if err != nil {
		i.UI.Error(err.Error())
		return 1
	}

	if typ == ibft.PoA && rawDeployment != "" {
		i.UI.Error(fmt.Sprintf("doesn't support deployment in PoA mode"))
		return 1
	}
	deployment, err := types.ParseUint64orHex(&rawDeployment)
	if err != nil {
		i.UI.Error(err.Error())
		return 1
	}

	from, err := types.ParseUint64orHex(&rawFrom)
	if err != nil {
		i.UI.Error(err.Error())
		return 1
	}
	if from <= 0 {
		i.UI.Error(`"from" must be positive number`)
		return 1
	}

	cc, err := chain.Import(genesisPath)
	if err != nil {
		i.UI.Error(err.Error())
		return 1
	}
	ibftSetting, ok := cc.Params.Engine["ibft"].(map[string]interface{})
	if !ok {
		i.UI.Error(fmt.Sprintf(`"ibft" setting doesn't exist in "engine" of genesis.json'`))
		return 1
	}

	if originalType, ok := ibftSetting["type"].(string); ok {
		// single setting
		delete(ibftSetting, "type")
		types := []map[string]interface{}{
			{
				"type": originalType,
				"from": 0,
				"to":   from - 1,
			},
		}
		newType := map[string]interface{}{
			"type": typ,
			"from": from,
		}
		if typ == ibft.PoS {
			newType["deployment"] = deployment
		}
		ibftSetting["types"] = append(types, newType)
	} else if types, ok := ibftSetting["types"].([]interface{}); ok {
		fmt.Println(types)
		lastType, ok := types[len(types)-1].(map[string]interface{})
		if !ok {
			i.UI.Error(`invalid type in "types"`)
			return 1
		}
		types[len(types)-1] = map[string]interface{}{
			"type": lastType["type"],
			"from": lastType["from"],
			"to":   from - 1,
		}

		newType := map[string]interface{}{
			"type": typ,
			"from": from,
		}
		if typ == ibft.PoS {
			newType["deployment"] = deployment
		}

		ibftSetting["types"] = append(types, newType)
	} else {
		i.UI.Error(fmt.Sprintf(`invalid settings in "engine"`))
		return 1
	}

	if err := os.Remove(genesisPath); err != nil {
		i.UI.Error(err.Error())
		return 1
	}
	if err = helper.WriteGenesisToDisk(cc, genesisPath); err != nil {
		i.UI.Error(err.Error())
		return 1
	}

	return 0
}
