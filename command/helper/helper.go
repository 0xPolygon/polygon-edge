package helper

import (
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/output"
	ibftOp "github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/server/proto"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/ryanuber/columnize"
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
	return cmd.Flag(command.GRPCAddressFlag).Value.String()
}

// GetJSONRPCAddress extracts the set JSON-RPC address
func GetJSONRPCAddress(cmd *cobra.Command) string {
	return cmd.Flag(command.JSONRPCFlag).Value.String()
}

// RegisterJSONOutputFlag registers the --json output setting for all child commands
func RegisterJSONOutputFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool(
		command.JSONOutputFlag,
		false,
		"the JSON-RPC interface",
	)
}

// RegisterGRPCAddressFlag registers the base GRPC address flag for all child commands
func RegisterGRPCAddressFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String(
		command.GRPCAddressFlag,
		fmt.Sprintf("%s:%d", "127.0.0.1", server.DefaultGRPCPort),
		"the GRPC interface",
	)
}

// ParseGRPCAddress parses the passed in GRPC address
func ParseGRPCAddress(grpcAddress string) (*net.TCPAddr, error) {
	return net.ResolveTCPAddr("tcp", grpcAddress)
}

// RegisterJSONRPCFlag registers the base JSON-RPC address flag for all child commands
func RegisterJSONRPCFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String(
		command.JSONRPCFlag,
		fmt.Sprintf("http://%s:%d", "127.0.0.1", server.DefaultJSONRPCPort),
		"the JSON-RPC interface",
	)
}

// ParseJSONRPCAddress parses the passed in JSONRPC address
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

// MultiAddrFromDNS constructs a multiAddr from the passed in DNS address and port combination
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
		return nil, errors.New("invalid DNS address")
	}

	s := strings.Trim(addr, "/")
	split := strings.Split(s, "/")

	if len(split) != 2 {
		return nil, errors.New("invalid DNS address")
	}

	switch split[0] {
	case "dns":
		version = "dns"
	case "dns4":
		version = "dns4"
	case "dns6":
		version = "dns6"
	default:
		return nil, errors.New("invalid DNS version")
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
