package server

import (
	"fmt"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"os"
)

func getGenerateConfigCmd() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "export",
		Short: "export default-config.yaml file with default parameters that can be used to run the server",
		Run:   runGenerateConfigCommand,
	}

	return configCmd
}

func runGenerateConfigCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)

	if err := generateConfig(*DefaultConfig()); err != nil {
		outputter.SetError(err)
		outputter.WriteOutput()
	}
}

func generateConfig(config Config) error {
	config.Network.MaxPeers = -1
	config.Network.MaxInboundPeers = -1
	config.Network.MaxOutboundPeers = -1

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("could not marshal Config struct")
	}

	if err := os.WriteFile("default-config.yaml", data, os.ModePerm); err != nil {
		return fmt.Errorf("could not create and write default-config.yaml file")
	}

	return nil
}
