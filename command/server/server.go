package server

import (
	"fmt"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/output"
	"github.com/spf13/cobra"
	"strconv"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/server"
)

func GetCommand() *cobra.Command {
	serverCmd := &cobra.Command{
		Use:     "server",
		Short:   "The default command that starts the Polygon Edge client, by bootstrapping all modules together",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	helper.RegisterGRPCAddressFlag(serverCmd)
	helper.RegisterJSONRPCFlag(serverCmd)

	setFlags(serverCmd)

	return serverCmd
}

func setFlags(cmd *cobra.Command) {
	defaultConfig := DefaultConfig()

	cmd.Flags().StringVar(
		&params.rawConfig.LogLevel,
		command.LogLevelFlag,
		defaultConfig.LogLevel,
		fmt.Sprintf(
			"the log level for console output. Default: %s",
			defaultConfig.LogLevel,
		),
	)

	cmd.Flags().StringVar(
		&params.rawConfig.GenesisPath,
		genesisPathFlag,
		defaultConfig.GenesisPath,
		fmt.Sprintf(
			"the genesis file used for starting the chain. Default: %s",
			defaultConfig.GenesisPath,
		),
	)

	cmd.Flags().StringVar(
		&params.configPath,
		configFlag,
		"",
		"the path to the CLI config. Supports .json and .hcl",
	)

	cmd.Flags().StringVar(
		&params.rawConfig.DataDir,
		dataDirFlag,
		defaultConfig.DataDir,
		fmt.Sprintf(
			"the data directory used for storing Polygon Edge client data. Default: %s",
			defaultConfig.DataDir,
		),
	)

	cmd.Flags().StringVar(
		&params.rawConfig.Network.Libp2pAddr,
		libp2pAddressFlag,
		fmt.Sprintf("127.0.0.1:%d", network.DefaultLibp2pPort),
		fmt.Sprintf(
			"the address and port for the libp2p service (address:port). Default: address: 127.0.0.1:%d",
			network.DefaultLibp2pPort,
		),
	)

	cmd.Flags().StringVar(
		&params.rawConfig.Telemetry.PrometheusAddr,
		prometheusAddressFlag,
		"",
		"the address and port for the prometheus instrumentation service (address:port)",
	)

	cmd.Flags().StringVar(
		&params.rawConfig.Network.NatAddr,
		natFlag,
		"",
		"the external IP address without port, as can be seen by peers",
	)

	cmd.Flags().StringVar(
		&params.rawConfig.Network.DNSAddr,
		dnsFlag,
		"",
		"the host DNS address which can be used by a remote peer for connection",
	)

	cmd.Flags().StringVar(
		&params.rawConfig.BlockGasTarget,
		blockGasTargetFlag,
		strconv.FormatUint(0, 10),
		"the target block gas limit for the chain. If omitted, the value of the parent block is used",
	)

	cmd.Flags().StringVar(
		&params.rawConfig.SecretsConfigPath,
		secretsConfigFlag,
		"",
		"the path to the SecretsManager config file. Used for Hashicorp Vault. "+
			"If omitted, the local FS secrets manager is used",
	)

	cmd.Flags().StringVar(
		&params.rawConfig.RestoreFile,
		restoreFlag,
		"",
		"the path to the archive blockchain data to restore on initialization",
	)

	cmd.Flags().BoolVar(
		&params.rawConfig.ShouldSeal,
		sealFlag,
		true,
		"the flag indicating that the client should seal blocks. Default: true",
	)

	cmd.Flags().BoolVar(
		&params.rawConfig.Network.NoDiscover,
		command.NoDiscoverFlag,
		defaultConfig.Network.NoDiscover,
		fmt.Sprintf(
			"prevent the client from discovering other peers. Default: %t",
			defaultConfig.Network.NoDiscover,
		),
	)

	cmd.Flags().Int64Var(
		&params.rawConfig.Network.MaxPeers,
		maxPeersFlag,
		-1,
		fmt.Sprintf(
			"the client's max number of peers allowed. Default: %d",
			defaultConfig.Network.MaxPeers,
		),
	)

	cmd.Flags().Int64Var(
		&params.rawConfig.Network.MaxInboundPeers,
		maxInboundPeersFlag,
		-1,
		fmt.Sprintf(
			"the client's max number of inbound peers allowed. Default: %d",
			defaultConfig.Network.MaxInboundPeers,
		),
	)

	cmd.Flags().Int64Var(
		&params.rawConfig.Network.MaxOutboundPeers,
		maxOutboundPeersFlag,
		-1,
		fmt.Sprintf(
			"the client's max number of outbound peers allowed. Default: %d",
			defaultConfig.Network.MaxOutboundPeers,
		),
	)

	cmd.Flags().Uint64Var(
		&params.rawConfig.TxPool.PriceLimit,
		priceLimitFlag,
		0,
		fmt.Sprintf(
			"the minimum gas price limit to enforce for acceptance into the pool. Default: %d",
			defaultConfig.TxPool.PriceLimit,
		),
	)

	cmd.Flags().Uint64Var(
		&params.rawConfig.TxPool.MaxSlots,
		maxSlotsFlag,
		command.DefaultMaxSlots,
		fmt.Sprintf(
			"maximum slots in the pool. Default: %d",
			command.DefaultMaxSlots,
		),
	)

	cmd.Flags().Uint64Var(
		&params.rawConfig.BlockTime,
		blockTimeFlag,
		defaultConfig.BlockTime,
		fmt.Sprintf(
			"minimum block time in seconds. Default: %ds",
			defaultConfig.BlockTime,
		),
	)

	cmd.Flags().StringArrayVar(
		&params.corsAllowedOrigins,
		corsOriginFlag,
		defaultConfig.Headers.AccessControlAllowOrigins,
		"the CORS header indicating whether any JSON-RPC response can be shared with the specified origin",
	)

	setDevFlags(cmd)
}

func setDevFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(
		&params.isDevMode,
		devFlag,
		false,
		"should the client start in dev mode. Default: false",
	)

	_ = cmd.Flags().MarkHidden(devFlag)

	cmd.Flags().Uint64Var(
		&params.devInterval,
		devIntervalFlag,
		0,
		"the client's dev notification interval. Default: 1s",
	)

	_ = cmd.Flags().MarkHidden(devIntervalFlag)
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	// Check if the config file has been specified
	if isConfigFileSpecified(cmd) {
		if err := params.initConfigFromFile(); err != nil {
			return err
		}
	}

	if err := params.validateFlags(); err != nil {
		return err
	}

	params.setRawGRPCAddress(helper.GetGRPCAddress(cmd))
	params.setRawJSONRPCAddress(helper.GetJSONRPCAddress(cmd))

	if err := params.initRawParams(); err != nil {
		return err
	}

	return nil
}

func isConfigFileSpecified(cmd *cobra.Command) bool {
	return cmd.Flags().Changed(configFlag)
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := output.InitializeOutputter(cmd)

	if err := runServerLoop(params.generateConfig(), outputter); err != nil {
		outputter.SetError(err)
		outputter.WriteOutput()

		return
	}
}

func runServerLoop(
	config *server.Config,
	outputter output.OutputFormatter,
) error {
	serverInstance, err := server.NewServer(config)
	if err != nil {
		return err
	}

	return helper.HandleSignals(serverInstance.Close, outputter)
}
