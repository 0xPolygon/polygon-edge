package export

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/server/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func GetCommand() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "export",
		Short: "export default-config.yaml file with default parameters that can be used to run the server",
		Run:   runGenerateConfigCommand,
	}

	setFlags(configCmd)

	return configCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&paramFlagValues.FileType,
		fileTypeFlag,
		"yaml",
		"file type of exported config file (yaml or json)",
	)
}

func runGenerateConfigCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := generateConfig(*config.DefaultConfig()); err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(&cmdResult{
		CommandOutput: "Configuration file successfully exported",
	})
}

func generateConfig(config config.Config) error {
	config.Network.MaxPeers = -1
	config.Network.MaxInboundPeers = -1
	config.Network.MaxOutboundPeers = -1

	var (
		data []byte
		err  error
	)

	switch paramFlagValues.FileType {
	case "yaml", "yml":
		data, err = yaml.Marshal(config)
	case "json":
		data, err = json.MarshalIndent(config, "", "    ")
	default:
		return errors.New("invalid file type, only yaml and json are supported")
	}

	if err != nil {
		return fmt.Errorf("could not marshal config struct, %w", err)
	}

	if err := os.WriteFile(
		fmt.Sprintf("default-config.%s", paramFlagValues.FileType),
		data,
		os.ModePerm); err != nil {
		return errors.New("could not create and write config file")
	}

	return nil
}
