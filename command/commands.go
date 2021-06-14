package command

import (
	"flag"
	"fmt"
	"os"

	"github.com/0xPolygon/minimal/command/server"
	"github.com/0xPolygon/minimal/minimal"
	"github.com/0xPolygon/minimal/types"
	"github.com/mitchellh/cli"
	"github.com/ryanuber/columnize"

	"google.golang.org/grpc"
)

// Commands returns a mapping of all available commands
func Commands() map[string]cli.CommandFactory {
	ui := &cli.BasicUi{
		Reader:      os.Stdin,
		Writer:      os.Stdout,
		ErrorWriter: os.Stderr,
	}

	meta := Meta{
		UI: ui,
	}

	return map[string]cli.CommandFactory{

		// GENERIC SDK COMMANDS //

		"server": func() (cli.Command, error) {
			return &server.Command{
				UI: ui,
			}, nil
		},
		// TODO The task to implement the dev command should start here
		//"dev": func() (cli.Command, error) {
		//	return &DevCommand{
		//		UI: ui,
		//	}, nil
		//},
		"genesis": func() (cli.Command, error) {
			return &GenesisCommand{
				UI: ui,
			}, nil
		},

		// PEER COMMANDS //

		"peers-add": func() (cli.Command, error) {
			return &PeersAdd{
				Meta: meta,
			}, nil
		},
		"peers-status": func() (cli.Command, error) {
			return &PeersStatus{
				Meta: meta,
			}, nil
		},
		"peers-list": func() (cli.Command, error) {
			return &PeersList{
				Meta: meta,
			}, nil
		},

		// IBFT COMMANDS //

		"ibft-init": func() (cli.Command, error) {
			return &IbftInit{
				Meta: meta,
			}, nil
		},
		"ibft-snapshot": func() (cli.Command, error) {
			return &IbftSnapshot{
				Meta: meta,
			}, nil
		},
		"ibft-candidates": func() (cli.Command, error) {
			return &IbftCandidates{
				Meta: meta,
			}, nil
		},
		"ibft-propose": func() (cli.Command, error) {
			return &IbftPropose{
				Meta: meta,
			}, nil
		},
		"ibft-status": func() (cli.Command, error) {
			return &IbftStatus{
				Meta: meta,
			}, nil
		},

		// TXPOOL COMMANDS //

		"txpool-add": func() (cli.Command, error) {
			return &TxPoolAdd{
				Meta: meta,
			}, nil
		},
		"txpool-status": func() (cli.Command, error) {
			return &TxPoolStatus{
				Meta: meta,
			}, nil
		},

		// BLOCKCHAIN COMMANDS //

		"status": func() (cli.Command, error) {
			return &StatusCommand{
				Meta: meta,
			}, nil
		},
		"monitor": func() (cli.Command, error) {
			return &MonitorCommand{
				Meta: meta,
			}, nil
		},
		"version": func() (cli.Command, error) {
			return &VersionCommand{
				UI: ui,
			}, nil
		},
	}
}

// MetaFlagDescriptor contains the description elements for a command flag. Implements types.FlagDescriptor
type MetaFlagDescriptor struct {
	description       string   // Flag description
	arguments         []string // Arguments list
	argumentsOptional bool     // Flag indicating if flag arguments are optional
	flagOptional      bool
}

func (m MetaFlagDescriptor) GetDescription() string {
	return m.description
}

func (m MetaFlagDescriptor) GetArgumentsList() []string {
	return m.arguments
}

func (m MetaFlagDescriptor) GetArgumentsOptional() bool {
	return m.argumentsOptional
}

func (m MetaFlagDescriptor) GetFlagOptional() bool {
	return m.flagOptional
}

type HelpGenerator interface {
	DefineFlags()
}

// Meta is a helper utility for the commands
type Meta struct {
	UI   cli.Ui
	addr string

	flagMap        map[string]types.FlagDescriptor
	hasGlobalFlags bool
}

// DefineFlags sets global flags used by several commands
func (m *Meta) DefineFlags() {
	m.hasGlobalFlags = true
	m.flagMap = make(map[string]types.FlagDescriptor)

	m.flagMap["grpc-address"] = MetaFlagDescriptor{
		description: fmt.Sprintf("Address of the gRPC API. Default: %s:%d", "127.0.0.1", minimal.DefaultGRPCPort),
		arguments: []string{
			"GRPC_ADDRESS",
		},
		argumentsOptional: false,
		flagOptional:      true,
	}
}

// FlagSet adds some default commands to handle grpc connections with the server
func (m *Meta) FlagSet(n string) *flag.FlagSet {
	f := flag.NewFlagSet(n, flag.ContinueOnError)
	f.StringVar(&m.addr, "grpc-address", fmt.Sprintf("%s:%d", "127.0.0.1", minimal.DefaultGRPCPort), "")

	return f
}

// Conn returns a grpc connection
func (m *Meta) Conn() (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(m.addr, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %v", err)
	}

	return conn, nil
}

// OUTPUT FORMATTING //

// formatList formats a list, using a specific blank value replacement
func formatList(in []string) string {
	columnConf := columnize.DefaultConfig()
	columnConf.Empty = "<none>"

	return columnize.Format(in, columnConf)
}

// formatKV formats key value pairs:
//
// Key = Value
//
// Key = <none>
func formatKV(in []string) string {
	columnConf := columnize.DefaultConfig()
	columnConf.Empty = "<none>"
	columnConf.Glue = " = "

	return columnize.Format(in, columnConf)
}
