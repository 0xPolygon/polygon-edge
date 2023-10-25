package helper

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command"
	ibftOp "github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/server/proto"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/ryanuber/columnize"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var ErrBlockTrackerPollInterval = errors.New("block tracker poll interval must be greater than 0")

type ClientCloseResult struct {
	Message string `json:"message"`
}

func (r *ClientCloseResult) GetOutput() string {
	return r.Message
}

type IPBinding string

const (
	LocalHostBinding     IPBinding = "127.0.0.1"
	AllInterfacesBinding IPBinding = "0.0.0.0"
)

// HandleSignals is a helper method for handling signals sent to the console
// Like stop, error, etc.
func HandleSignals(
	closeFn func(),
	outputter command.OutputFormatter,
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
	if cmd.Flags().Changed(command.GRPCAddressFlagLEGACY) {
		// The legacy GRPC flag was set, use that value
		return cmd.Flag(command.GRPCAddressFlagLEGACY).Value.String()
	}

	return cmd.Flag(command.GRPCAddressFlag).Value.String()
}

// GetJSONRPCAddress extracts the set JSON-RPC address
func GetJSONRPCAddress(cmd *cobra.Command) string {
	return cmd.Flag(command.JSONRPCFlag).Value.String()
}

// GetJSONLogFormat extracts the set JSON Format flag
func GetJSONLogFormat(cmd *cobra.Command) bool {
	return cmd.Flag(command.JSONOutputFlag).Changed
}

// RegisterJSONOutputFlag registers the --json output setting for all child commands
func RegisterJSONOutputFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool(
		command.JSONOutputFlag,
		false,
		"get all outputs in json format (default false)",
	)
}

// RegisterGRPCAddressFlag registers the base GRPC address flag for all child commands
func RegisterGRPCAddressFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String(
		command.GRPCAddressFlag,
		fmt.Sprintf("%s:%d", LocalHostBinding, server.DefaultGRPCPort),
		"the GRPC interface",
	)
}

// RegisterLegacyGRPCAddressFlag registers the legacy GRPC address flag for all child commands
func RegisterLegacyGRPCAddressFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String(
		command.GRPCAddressFlagLEGACY,
		fmt.Sprintf("%s:%d", LocalHostBinding, server.DefaultGRPCPort),
		"the GRPC interface",
	)

	// Mark the legacy grpc flag as hidden
	_ = cmd.PersistentFlags().MarkHidden(command.GRPCAddressFlagLEGACY)
}

// ParseGRPCAddress parses the passed in GRPC address
func ParseGRPCAddress(grpcAddress string) (*net.TCPAddr, error) {
	return net.ResolveTCPAddr("tcp", grpcAddress)
}

// RegisterJSONRPCFlag registers the base JSON-RPC address flag for all child commands
func RegisterJSONRPCFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String(
		command.JSONRPCFlag,
		fmt.Sprintf("http://%s:%d", AllInterfacesBinding, server.DefaultJSONRPCPort),
		"the JSON-RPC interface",
	)
}

// ParseJSONRPCAddress parses the passed in JSONRPC address
func ParseJSONRPCAddress(jsonrpcAddress string) (*url.URL, error) {
	return url.ParseRequestURI(jsonrpcAddress)
}

// ResolveAddr resolves the passed in TCP address
// The second param is the default ip to bind to, if no ip address is specified
func ResolveAddr(address string, defaultIP IPBinding) (*net.TCPAddr, error) {
	addr, err := net.ResolveTCPAddr("tcp", address)

	if err != nil {
		return nil, fmt.Errorf("failed to parse addr '%s': %w", address, err)
	}

	if addr.IP == nil {
		addr.IP = net.ParseIP(string(defaultIP))
	}

	return addr, nil
}

// WriteGenesisConfigToDisk writes the passed in configuration to a genesis file at the specified path
func WriteGenesisConfigToDisk(genesisConfig *chain.Chain, genesisPath string) error {
	data, err := json.MarshalIndent(genesisConfig, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to generate genesis: %w", err)
	}

	if err := common.SaveFileSafe(genesisPath, data, 0660); err != nil {
		return fmt.Errorf("failed to write genesis: %w", err)
	}

	return nil
}

func SetRequiredFlags(cmd *cobra.Command, requiredFlags []string) {
	for _, requiredFlag := range requiredFlags {
		_ = cmd.MarkFlagRequired(requiredFlag)
	}
}

func ParseAmount(amount string) (*big.Int, error) {
	result, ok := new(big.Int).SetString(amount, 0)
	if !ok || result.Cmp(big.NewInt(0)) <= 0 {
		return nil, fmt.Errorf("amount %s should be numerical value greater than zero", amount)
	}

	return result, nil
}

func ValidateProxyContractsAdmin(proxyContractsAdmin string) error {
	if err := types.IsValidAddress(proxyContractsAdmin); err != nil {
		return fmt.Errorf("proxy contracts admin address is not valid: %w", err)
	}

	proxyContractsAdminAddr := types.StringToAddress(proxyContractsAdmin)
	if proxyContractsAdminAddr == types.ZeroAddress {
		return errors.New("proxy contracts admin address must not be zero address")
	}

	return nil
}

type PremineInfo struct {
	Address types.Address
	Amount  *big.Int
	Key     string // only used for tests
}

// parsePremineInfo parses provided premine information and returns premine address and amount
func ParsePremineInfo(premineInfoRaw string) (*PremineInfo, error) {
	var (
		address types.Address
		amount  = command.DefaultPremineBalance
		key     string
		err     error
	)

	parts := strings.Split(premineInfoRaw, ":")

	if len(parts) > 1 { // <addr>:<balance>
		amount, err = common.ParseUint256orHex(&parts[1])
		if err != nil {
			return nil, fmt.Errorf("failed to parse amount %s: %w", parts[1], err)
		}

		address = types.StringToAddress(parts[0])

		if len(parts) == 3 { // <addr>:<balance>:<key>
			key = parts[2]
		}
	} else {
		// <addr>
		address = types.StringToAddress(premineInfoRaw)
	}

	return &PremineInfo{Address: address, Amount: amount, Key: key}, nil
}
