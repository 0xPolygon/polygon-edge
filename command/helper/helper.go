package helper

import (
	"errors"
	"flag"
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/output"
	server2 "github.com/0xPolygon/polygon-edge/command/server"
	"github.com/0xPolygon/polygon-edge/consensus/ibft"
	ibftOp "github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/server/proto"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/staking"
	"github.com/0xPolygon/polygon-edge/types"
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

type ClientCloseResult struct {
	Message string `json:"message"`
}

func (r *ClientCloseResult) GetOutput() string {
	return r.Message
}

// HandleSignals is a helper method for handling signals sent to the console
// Like stop, error, etc.
func HandleSignals(
	closeFn func(),
	outputter output.OutputFormatter,
) error {
	signalCh := common.GetTerminationSignalCh()
	sig := <-signalCh

	closeMessage := fmt.Sprintf("\n[SIGNAL] Caught signal: %v\n", sig)
	closeMessage += "Gracefully shutting down client...\n"

	outputter.SetCommandResult(
		&ClientCloseResult{
			Message: closeMessage,
		},
	)
	outputter.WriteOutput()

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
		return errors.New("shutdown by signal channel")
	case <-time.After(5 * time.Second):
		return errors.New("shutdown by timeout")
	case <-gracefulCh:
		return nil
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
	premine []string,
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
	premine   []string
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
func BootstrapDevCommand(baseCommand string, args []string) (*server2.Config, error) {
	config := server2.DefaultConfig()

	cliConfig := &server2.Config{
		Network: &server2.Network{
			NoDiscover:       true,
			MaxOutboundPeers: 0,
			MaxInboundPeers:  0,
		},
		TxPool:    &server2.TxPool{},
		Telemetry: &server2.Telemetry{},
	}
	cliConfig.ShouldSeal = true
	cliConfig.GenesisPath = "genesis.json"

	flags := flag.NewFlagSet(baseCommand, flag.ContinueOnError)
	flags.Usage = func() {}

	var (
		premine  helperFlags.ArrayFlags
		gaslimit uint64
		chainID  uint64
	)

	flags.StringVar(&cliConfig.LogLevel, "log-level", server2.DefaultConfig().LogLevel, "")
	flags.Var(&premine, "premine", "")
	flags.Uint64Var(&cliConfig.TxPool.PriceLimit, "price-limit", 0, "")
	flags.Uint64Var(&cliConfig.TxPool.MaxSlots, "max-slots", DefaultMaxSlots, "")
	flags.Uint64Var(&gaslimit, "block-gas-limit", GenesisGasLimit, "")
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
		chainName: config.GenesisPath,
		premine:   premine,
		gasLimit:  gaslimit,
		chainID:   chainID,
	}); err != nil {
		return nil, err
	}

	return config, nil
}

func ReadConfig(baseCommand string, args []string) (*server2.Config, error) {
	config := server2.DefaultConfig()

	cliConfig := &server2.Config{
		Network:   &server2.Network{},
		TxPool:    &server2.TxPool{},
		Telemetry: &server2.Telemetry{},
	}

	flags := flag.NewFlagSet(baseCommand, flag.ContinueOnError)
	flags.Usage = func() {}

	var configFile string

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
		diskConfigFile, err := server2.readConfigFile(configFile)
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

// ResolveAddr resolves the passed in TCP address
func ResolveAddr(address string) (*net.TCPAddr, error) {
	addr, err := net.ResolveTCPAddr("tcp", address)

	if err != nil {
		return nil, fmt.Errorf("failed to parse addr '%s': %w", address, err)
	}

	if addr.IP == nil {
		addr.IP = net.ParseIP("127.0.0.1")
	}

	return addr, nil
}

func MultiAddrFromDNS(addr string, port int) (multiaddr.Multiaddr, error) {
	var (
		version string
		domain  string
	)

	match, err := regexp.MatchString(
		"^/?(dns)(4|6)?/[^-|^/][A-Za-z0-9-]([^-|^/]?)+([\\-\\.]{1}[a-z0-9]+)*\\.[A-Za-z]{2,6}(/?)$",
		addr,
	)
	if err != nil || !match {
		return nil, errors.New("invalid DNSAddr address")
	}

	s := strings.Trim(addr, "/")
	split := strings.Split(s, "/")

	if len(split) != 2 {
		return nil, errors.New("invalid DNSAddr address")
	}

	switch split[0] {
	case "dns":
		version = "dns"
	case "dns4":
		version = "dns4"
	case "dns6":
		version = "dns6"
	default:
		return nil, errors.New("invalid DNSAddr version")
	}

	domain = split[1]

	multiAddr, err := multiaddr.NewMultiaddr(
		fmt.Sprintf(
			"/%s/%s/tcp/%d",
			version,
			domain,
			port,
		),
	)

	if err != nil {
		return nil, errors.New("could not create a multi address")
	}

	return multiAddr, nil
}
