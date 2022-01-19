package helper

import (
	"encoding/json"
	"flag"
	"fmt"

	"github.com/0xPolygon/polygon-edge/server"
	"github.com/mitchellh/cli"
	"google.golang.org/grpc"
)

// Flags are sets of flags for specific purpose and provide functions

// GRPCFlag is a helper utility for GRPC flag and provides gRPC connection
type GRPCFlag struct {
	Addr string
}

// DefineFlags sets some flags for grpc settings
func (g *GRPCFlag) DefineFlags(flagMap map[string]FlagDescriptor) {
	flagMap["grpc-address"] = FlagDescriptor{
		Description: fmt.Sprintf("Address of the gRPC API. Default: %s:%d", "127.0.0.1", server.DefaultGRPCPort),
		Arguments: []string{
			"GRPC_ADDRESS",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}
}

// FlagSet adds some default commands to handle grpc connections with the server
func (g *GRPCFlag) FlagSet(f *flag.FlagSet) {
	f.StringVar(&g.Addr, "grpc-address", fmt.Sprintf("%s:%d", "127.0.0.1", server.DefaultGRPCPort), "")
}

// Conn returns a grpc connection
func (g *GRPCFlag) Conn() (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(g.Addr, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %w", err)
	}

	return conn, nil
}

// FormatterFlag is a helper utility for formatter
type FormatterFlag struct {
	UI     cli.Ui
	IsJSON bool
}

// DefineFlags sets some flags for json output
func (f *FormatterFlag) DefineFlags(flagMap map[string]FlagDescriptor) {
	flagMap["json"] = FlagDescriptor{
		Description: "Output JSON instead of human readable result",
		Arguments: []string{
			"JSON",
		},
		ArgumentsOptional: true,
		FlagOptional:      true,
	}
}

// FlagSet adds some default commands for json flag
func (f *FormatterFlag) FlagSet(fs *flag.FlagSet) {
	fs.BoolVar(&f.IsJSON, "json", false, "")
}

// OutputError is helper function to print output with different format
func (f *FormatterFlag) OutputError(e error) {
	if f.IsJSON {
		// marshal for escaped error
		errRes := struct {
			Err string `json:"error"`
		}{Err: e.Error()}

		bytes, err := json.Marshal(errRes)
		if err != nil {
			f.UI.Error(err.Error())
		} else {
			f.UI.Error(string(bytes))
		}
	} else {
		f.UI.Error(e.Error())
	}
}

// OutputResult is helper function to print result with different format
func (f *FormatterFlag) OutputResult(r CommandResult) {
	var out string

	var err error

	if f.IsJSON {
		var bytes []byte
		if bytes, err = json.Marshal(r); err == nil {
			out = string(bytes)
		}
	} else {
		out = r.Output()
	}

	if err != nil {
		f.OutputError(err)
	} else {
		f.UI.Output(out)
	}
}
