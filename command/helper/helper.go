package helper

import (
	"errors"
	"flag"
	"fmt"
	"github.com/0xPolygon/polygon-edge/consensus/ibft"
	ibftOp "github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/server/proto"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/helper/common"
	helperFlags "github.com/0xPolygon/polygon-edge/helper/flags"
	"github.com/0xPolygon/polygon-edge/helper/staking"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/mitchellh/cli"
	"github.com/ryanuber/columnize"
)

const (
	GenesisFileName       = "genesis.json"
	DefaultChainName      = "polygon-edge"
	DefaultChainID        = 100
	DefaultPremineBalance = "0x3635C9ADC5DEA00000" // 1000 ETH
	DefaultConsensus      = server.IBFTConsensus
	DefaultMaxSlots       = 4096
	GenesisGasUsed        = 458752  // 0x70000
	GenesisGasLimit       = 5242880 // 0x500000
)

const (
	JSONOutputFlag  = "json"
	GRPCAddressFlag = "grpc-address"
	JSONRPCFlag     = "jsonrpc"
)

// FlagDescriptor contains the description elements for a command flag
type FlagDescriptor struct {
	Description       string   // Flag description
	Arguments         []string // Arguments list
	ArgumentsOptional bool     // Flag indicating if flag arguments are optional
	FlagOptional      bool
}

// GetDescription gets the flag description
func (fd *FlagDescriptor) GetDescription() string {
	return fd.Description
}

// GetArgumentsList gets the list of arguments for the flag
func (fd *FlagDescriptor) GetArgumentsList() []string {
	return fd.Arguments
}

// AreArgumentsOptional checks if the flag arguments are optional
func (fd *FlagDescriptor) AreArgumentsOptional() bool {
	return fd.ArgumentsOptional
}

// IsFlagOptional checks if the flag itself is optional
func (fd *FlagDescriptor) IsFlagOptional() bool {
	return fd.FlagOptional
}

// GenerateHelp is a utility function called by every command's Help() method
func GenerateHelp(synopsys string, usage string, flagMap map[string]FlagDescriptor) string {
	helpOutput := ""

	flagCounter := 0

	for flagEl, descriptor := range flagMap {
		helpOutput += GenerateFlagDesc(flagEl, descriptor) + "\n"
		flagCounter++

		if flagCounter < len(flagMap) {
			helpOutput += "\n"
		}
	}

	if len(flagMap) > 0 {
		return fmt.Sprintf("Description:\n\n%s\n\nUsage:\n\n\t%s\n\nFlags:\n\n%s", synopsys, usage, helpOutput)
	} else {
		return fmt.Sprintf("Description:\n\n%s\n\nUsage:\n\n\t%s\n", synopsys, usage)
	}
}

// GenerateFlagDesc generates the flag descriptions in a readable format
func GenerateFlagDesc(flagEl string, descriptor FlagDescriptor) string {
	// Generate the top row (with various flags)
	topRow := fmt.Sprintf("--%s", flagEl)

	argumentsOptional := descriptor.AreArgumentsOptional()
	argumentsList := descriptor.GetArgumentsList()

	argLength := len(argumentsList)

	if argLength > 0 {
		topRow += " "
		if argumentsOptional {
			topRow += "["
		}

		for argIndx, argument := range argumentsList {
			topRow += argument

			if argIndx < argLength-1 && argLength > 1 {
				topRow += " "
			}
		}

		if argumentsOptional {
			topRow += "]"
		}
	}

	// Generate the bottom description
	bottomRow := fmt.Sprintf("\t%s", descriptor.GetDescription())

	return fmt.Sprintf("%s\n%s", topRow, bottomRow)
}

// GenerateUsage is a helper function for generating command usage text
func GenerateUsage(baseCommand string, flagMap map[string]FlagDescriptor) string {
	output := baseCommand + " "

	maxFlagsPerLine := 3 // Just an arbitrary value, can be anything reasonable

	var addedFlags int // Keeps track of when a newline character needs to be inserted

	for flagEl, descriptor := range flagMap {
		// Open the flag bracket
		if descriptor.IsFlagOptional() {
			output += "["
		}

		// Add the actual flag name
		output += fmt.Sprintf("--%s", flagEl)

		// Open the argument bracket
		if descriptor.AreArgumentsOptional() {
			output += " ["
		}

		argumentsList := descriptor.GetArgumentsList()

		// Add the flag arguments list
		for argIndex, argument := range argumentsList {
			if argIndex == 0 && !descriptor.AreArgumentsOptional() {
				// Only called for the first argument
				output += " "
			}

			output += argument

			if argIndex < len(argumentsList)-1 {
				output += " "
			}
		}

		// Close the argument bracket
		if descriptor.AreArgumentsOptional() {
			output += "]"
		}

		// Close the flag bracket
		if descriptor.IsFlagOptional() {
			output += "]"
		}

		addedFlags++
		if addedFlags%maxFlagsPerLine == 0 {
			output += "\n\t"
		} else {
			output += " "
		}
	}

	return output
}

// HandleSignals is a helper method for handling signals sent to the console
// Like stop, error, etc.
func HandleSignals(closeFn func(), ui cli.Ui) int {
	signalCh := common.GetTerminationSignalCh()
	sig := <-signalCh

	output := fmt.Sprintf("\n[SIGNAL] Caught signal: %v\n", sig)
	output += "Gracefully shutting down client...\n"

	ui.Output(output)

	// Call the Minimal server close callback
	gracefulCh := make(chan struct{})

	go func() {
		if closeFn != nil {
			closeFn()
		}

		close(gracefulCh)
	}()

	select {
	case <-signalCh:
		return 1
	case <-time.After(5 * time.Second):
		return 1
	case <-gracefulCh:
		return 0
	}
}

const (
	StatError   = "StatError"
	ExistsError = "ExistsError"
)

// GenesisGenError is a specific error type for generating genesis
type GenesisGenError struct {
	message   string
	errorType string
}

// GetMessage returns the message of the genesis generation error
func (g *GenesisGenError) GetMessage() string {
	return g.message
}

// GetType returns the type of the genesis generation error
func (g *GenesisGenError) GetType() string {
	return g.errorType
}

// VerifyGenesisExistence checks if the genesis file at the specified path is present
func VerifyGenesisExistence(genesisPath string) *GenesisGenError {
	_, err := os.Stat(genesisPath)
	if err != nil && !os.IsNotExist(err) {
		return &GenesisGenError{
			message:   fmt.Sprintf("failed to stat (%s): %v", genesisPath, err),
			errorType: StatError,
		}
	}

	if !os.IsNotExist(err) {
		return &GenesisGenError{
			message:   fmt.Sprintf("genesis file at path (%s) already exists", genesisPath),
			errorType: ExistsError,
		}
	}

	return nil
}

// FillPremineMap fills the premine map for the genesis.json file with passed in balances and accounts
func FillPremineMap(
	premineMap map[types.Address]*chain.GenesisAccount,
	premine helperFlags.ArrayFlags,
) error {
	for _, prem := range premine {
		var addr types.Address

		val := DefaultPremineBalance

		if indx := strings.Index(prem, ":"); indx != -1 {
			// <addr>:<balance>
			addr, val = types.StringToAddress(prem[:indx]), prem[indx+1:]
		} else {
			// <addr>
			addr = types.StringToAddress(prem)
		}

		amount, err := types.ParseUint256orHex(&val)
		if err != nil {
			return fmt.Errorf("failed to parse amount %s: %w", val, err)
		}

		premineMap[addr] = &chain.GenesisAccount{
			Balance: amount,
		}
	}

	return nil
}

// MergeMaps is a helper method for merging multiple maps.
// If two or more maps have the same keys, the map that is passed in later
// will have its key value override the previous same key values
func MergeMaps(maps ...map[string]interface{}) map[string]interface{} {
	mergedMap := make(map[string]interface{})

	for _, m := range maps {
		for key, value := range m {
			mergedMap[key] = value
		}
	}

	return mergedMap
}

func GetValidatorsFromPrefixPath(prefix string) ([]types.Address, error) {
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

type devGenesisParams struct {
	chainName string
	premine   helperFlags.ArrayFlags
	gasLimit  uint64
	chainID   uint64
}

// generateDevGenesis generates a base dev genesis file with premined balances
func generateDevGenesis(params devGenesisParams) error {
	genesisPath := filepath.Join("./", GenesisFileName)

	generateError := VerifyGenesisExistence(genesisPath)

	if generateError != nil {
		switch generateError.GetType() {
		case StatError:
			// Unable to stat file
			return errors.New(generateError.GetMessage())
		case ExistsError:
			// Not an error for the dev command, it shouldn't regenerate the genesis
			return nil
		}
	}

	cc := &chain.Chain{
		Name: params.chainName,
		Genesis: &chain.Genesis{
			GasLimit:   params.gasLimit,
			Difficulty: 1,
			Alloc:      map[types.Address]*chain.GenesisAccount{},
			ExtraData:  []byte{},
			GasUsed:    GenesisGasUsed,
		},
		Params: &chain.Params{
			ChainID: int(params.chainID),
			Forks:   chain.AllForksEnabled,
			Engine: map[string]interface{}{
				"dev": map[string]interface{}{},
			},
		},
		Bootnodes: []string{},
	}

	stakingAccount, err := staking.PredeployStakingSC(
		[]types.Address{},
	)
	if err != nil {
		return err
	}

	cc.Genesis.Alloc[staking.StakingSCAddress] = stakingAccount

	if err := FillPremineMap(cc.Genesis.Alloc, params.premine); err != nil {
		return err
	}

	//return WriteGenesisToDisk(cc, genesisPath)
	// TODO refactor

	return nil
}

// BootstrapDevCommand creates a config and generates the dev genesis file
func BootstrapDevCommand(baseCommand string, args []string) (*Config, error) {
	config := DefaultConfig()

	cliConfig := &Config{
		Network: &Network{
			NoDiscover:       true,
			MaxOutboundPeers: 0,
			MaxInboundPeers:  0,
		},
		TxPool:    &TxPool{},
		Telemetry: &Telemetry{},
	}
	cliConfig.Seal = true
	cliConfig.Dev = true
	cliConfig.Chain = "genesis.json"

	flags := flag.NewFlagSet(baseCommand, flag.ContinueOnError)
	flags.Usage = func() {}

	var (
		premine  helperFlags.ArrayFlags
		gaslimit uint64
		chainID  uint64
	)

	flags.StringVar(&cliConfig.LogLevel, "log-level", DefaultConfig().LogLevel, "")
	flags.Var(&premine, "premine", "")
	flags.Uint64Var(&cliConfig.TxPool.PriceLimit, "price-limit", 0, "")
	flags.Uint64Var(&cliConfig.TxPool.MaxSlots, "max-slots", DefaultMaxSlots, "")
	flags.Uint64Var(&gaslimit, "block-gas-limit", GenesisGasLimit, "")
	flags.Uint64Var(&cliConfig.DevInterval, "dev-interval", 0, "")
	flags.Uint64Var(&chainID, "chainid", DefaultChainID, "")
	flags.StringVar(&cliConfig.BlockGasTarget, "block-gas-target", strconv.FormatUint(0, 10), "")
	flags.StringVar(&cliConfig.RestoreFile, "restore", "", "")

	if err := flags.Parse(args); err != nil {
		return nil, err
	}

	if err := config.mergeConfigWith(cliConfig); err != nil {
		return nil, err
	}

	if err := generateDevGenesis(devGenesisParams{
		chainName: config.Chain,
		premine:   premine,
		gasLimit:  gaslimit,
		chainID:   chainID,
	}); err != nil {
		return nil, err
	}

	return config, nil
}

func ReadConfig(baseCommand string, args []string) (*Config, error) {
	config := DefaultConfig()

	cliConfig := &Config{
		Network:   &Network{},
		TxPool:    &TxPool{},
		Telemetry: &Telemetry{},
	}

	flags := flag.NewFlagSet(baseCommand, flag.ContinueOnError)
	flags.Usage = func() {}

	var configFile string

	flags.StringVar(&cliConfig.LogLevel, "log-level", "", "")
	flags.BoolVar(&cliConfig.Seal, "seal", false, "")
	flags.StringVar(&configFile, "config", "", "")
	flags.StringVar(&cliConfig.Chain, "chain", "", "")
	flags.StringVar(&cliConfig.DataDir, "data-dir", "", "")
	flags.StringVar(&cliConfig.GRPCAddr, "grpc", "", "")
	flags.StringVar(&cliConfig.JSONRPCAddr, "jsonrpc", "", "")
	flags.StringVar(&cliConfig.Join, "join", "", "")
	flags.StringVar(&cliConfig.Network.Addr, "libp2p", "", "")
	flags.StringVar(&cliConfig.Telemetry.PrometheusAddr, "prometheus", "", "")
	flags.StringVar(
		&cliConfig.Network.NatAddr,
		"nat",
		"",
		"the external IP address without port, as can be seen by peers",
	)
	flags.StringVar(
		&cliConfig.Network.DNS,
		"dns",
		"",
		" the host DNS address which can be used by a remote peer for connection",
	)
	flags.BoolVar(&cliConfig.Network.NoDiscover, "no-discover", false, "")
	flags.Int64Var(&cliConfig.Network.MaxPeers, "max-peers", -1, "maximum number of peers")
	flags.Int64Var(&cliConfig.Network.MaxInboundPeers, "max-inbound-peers", -1, "maximum number of inbound peers")
	flags.Int64Var(&cliConfig.Network.MaxOutboundPeers, "max-outbound-peers", -1, "maximum number of outbound peers")
	flags.Uint64Var(&cliConfig.TxPool.PriceLimit, "price-limit", 0, "")
	flags.Uint64Var(&cliConfig.TxPool.MaxSlots, "max-slots", DefaultMaxSlots, "")
	flags.BoolVar(&cliConfig.Dev, "dev", false, "")
	flags.Uint64Var(&cliConfig.DevInterval, "dev-interval", 1, "")
	flags.StringVar(&cliConfig.BlockGasTarget, "block-gas-target", strconv.FormatUint(0, 10), "")
	flags.StringVar(&cliConfig.Secrets, "secrets-config", "", "")
	flags.StringVar(&cliConfig.RestoreFile, "restore", "", "")
	flags.Uint64Var(&cliConfig.BlockTime, "block-time", config.BlockTime, "")

	if err := flags.Parse(args); err != nil {
		return nil, err
	}

	if cliConfig.Network.MaxPeers != -1 {
		if cliConfig.Network.MaxInboundPeers != -1 || cliConfig.Network.MaxOutboundPeers != -1 {
			return nil, errors.New("both max-peers and max-inbound/outbound flags are set")
		}
	}

	if configFile != "" {
		// A config file has been passed in, parse it
		diskConfigFile, err := readConfigFile(configFile)
		if err != nil {
			return nil, err
		}

		if diskConfigFile.Network.MaxPeers != -1 {
			if diskConfigFile.Network.MaxInboundPeers != -1 || diskConfigFile.Network.MaxOutboundPeers != -1 {
				return nil, errors.New("both max-peers & max-inbound/outbound flags are set")
			}
		}

		if err := config.mergeConfigWith(diskConfigFile); err != nil {
			return nil, err
		}
	}

	if err := config.mergeConfigWith(cliConfig); err != nil {
		return nil, err
	}

	return config, nil
}

type HelpGenerator interface {
	DefineFlags()
}

// OUTPUT FORMATTING //

// FormatList formats a list, using a specific blank value replacement
func FormatList(in []string) string {
	columnConf := columnize.DefaultConfig()
	columnConf.Empty = "<none>"

	return columnize.Format(in, columnConf)
}

// FormatKV formats key value pairs:
//
// Key = Value
//
// Key = <none>
func FormatKV(in []string) string {
	columnConf := columnize.DefaultConfig()
	columnConf.Empty = "<none>"
	columnConf.Glue = " = "

	return columnize.Format(in, columnConf)
}

// GetTxPoolClientConnection returns the TxPool operator client connection
func GetTxPoolClientConnection(address string) (
	txpoolOp.TxnPoolOperatorClient,
	error,
) {
	conn, err := GetGRPCConnection(address)
	if err != nil {
		return nil, err
	}

	return txpoolOp.NewTxnPoolOperatorClient(conn), nil
}

// GetSystemClientConnection returns the System operator client connection
func GetSystemClientConnection(address string) (
	proto.SystemClient,
	error,
) {
	conn, err := GetGRPCConnection(address)
	if err != nil {
		return nil, err
	}

	return proto.NewSystemClient(conn), nil
}

// GetIBFTOperatorClientConnection returns the IBFT operator client connection
func GetIBFTOperatorClientConnection(address string) (
	ibftOp.IbftOperatorClient,
	error,
) {
	conn, err := GetGRPCConnection(address)
	if err != nil {
		return nil, err
	}

	return ibftOp.NewIbftOperatorClient(conn), nil
}

// GetGRPCConnection returns a grpc client connection
func GetGRPCConnection(address string) (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %w", err)
	}

	return conn, nil
}

// GetGRPCAddress extracts the set GRPC address
func GetGRPCAddress(cmd *cobra.Command) string {
	return cmd.Flag(GRPCAddressFlag).Value.String()
}

// GetJSONRPCAddress extracts the set JSON-RPC address
func GetJSONRPCAddress(cmd *cobra.Command) string {
	return cmd.Flag(JSONRPCFlag).Value.String()
}

// RegisterJSONOutputFlag registers the --json output setting for all child commands
func RegisterJSONOutputFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool(
		JSONOutputFlag,
		false,
		"the JSON-RPC interface",
	)
}

// RegisterGRPCAddressFlag registers the base GRPC address flag for all child commands
func RegisterGRPCAddressFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String(
		GRPCAddressFlag,
		fmt.Sprintf("%s:%d", "127.0.0.1", server.DefaultGRPCPort),
		"the GRPC interface",
	)
}

func ParseGRPCAddress(grpcAddress string) (*net.TCPAddr, error) {
	return net.ResolveTCPAddr("tcp", grpcAddress)
}

// RegisterJSONRPCFlag registers the base JSON-RPC address flag for all child commands
func RegisterJSONRPCFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String(
		JSONRPCFlag,
		fmt.Sprintf("http://%s:%d", "127.0.0.1", server.DefaultJSONRPCPort),
		"the JSON-RPC interface",
	)
}

func ParseJSONRPCAddress(jsonrpcAddress string) (*url.URL, error) {
	return url.ParseRequestURI(jsonrpcAddress)
}

func GetInterruptCh() chan os.Signal {
	// wait for the user to quit with ctrl-c
	signalCh := make(chan os.Signal, 4)
	signal.Notify(
		signalCh,
		os.Interrupt,
		syscall.SIGTERM,
		syscall.SIGHUP,
	)

	return signalCh
}
