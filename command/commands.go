package command

import (
	"flag"
	"fmt"
	"os"

	"github.com/0xPolygon/minimal/command/server"
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
		"dev": func() (cli.Command, error) {
			return &DevCommand{
				UI: ui,
			}, nil
		},
		"genesis": func() (cli.Command, error) {
			return &GenesisCommand{
				UI: ui,
			}, nil
		},

		// PEER COMMANDS //

		"peers add": func() (cli.Command, error) {
			return &PeersAdd{
				Meta: meta,
			}, nil
		},
		"peers status": func() (cli.Command, error) {
			return &PeersStatus{
				Meta: meta,
			}, nil
		},
		"peers list": func() (cli.Command, error) {
			return &PeersList{
				Meta: meta,
			}, nil
		},

		// IBFT COMMANDS //

		"ibft init": func() (cli.Command, error) {
			return &IbftInit{
				Meta: meta,
			}, nil
		},
		"ibft snapshot": func() (cli.Command, error) {
			return &IbftSnapshot{
				Meta: meta,
			}, nil
		},
		"ibft candidates": func() (cli.Command, error) {
			return &IbftCandidates{
				Meta: meta,
			}, nil
		},
		"ibft propose": func() (cli.Command, error) {
			return &IbftPropose{
				Meta: meta,
			}, nil
		},
		"ibft status": func() (cli.Command, error) {
			return &IbftStatus{
				Meta: meta,
			}, nil
		},

		// TXPOOL COMMANDS //

		"txpool add": func() (cli.Command, error) {
			return &TxPoolAdd{
				Meta: meta,
			}, nil
		},
		"txpool status": func() (cli.Command, error) {
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

// FlagDescriptor contains the description elements for a command flag
type FlagDescriptor struct {
	description       string   // Flag description
	arguments         []string // Arguments list
	argumentsOptional bool     // Flag indicating if flag arguments are optional
}

type HelpGenerator interface {
	DefineFlags()
}

// Meta is a helper utility for the commands
type Meta struct {
	HelpGenerator

	UI   cli.Ui
	addr string

	flagMap    map[string]FlagDescriptor
	helperText string
}

// GenerateHelp is a utility function called by every command's Help() method
func (m *Meta) GenerateHelp(synopsys string, usage string) string {
	helpOutput := ""

	flagCounter := 0
	for flagEl, descriptor := range m.flagMap {
		helpOutput += m.GenerateFlagDesc(flagEl, descriptor) + "\n"
		flagCounter++

		if flagCounter < len(m.flagMap) {
			helpOutput += "\n"
		}
	}

	if len(m.flagMap) > 0 {
		return fmt.Sprintf("Description:\n\n%s\n\nUsage:\n\n\t%s\n\nFlags:\n\n%s", synopsys, usage, helpOutput)
	} else {
		return fmt.Sprintf("Description:\n\n%s\n\nUsage:\n\n\t%s\n", synopsys, usage)
	}
}

// GenerateFlagDesc generates the flag descriptions in a readable format
func (m *Meta) GenerateFlagDesc(flagEl string, descriptor FlagDescriptor) string {
	// Generate the top row (with various flags)
	topRow := fmt.Sprintf("--%s", flagEl)

	argLength := len(descriptor.arguments)

	if argLength > 0 {
		topRow += " "
		if descriptor.argumentsOptional {
			topRow += "["
		}

		for argIndx, argument := range descriptor.arguments {
			topRow += argument

			if argIndx < argLength-1 && argLength > 1 {
				topRow += " "
			}
		}

		if descriptor.argumentsOptional {
			topRow += "]"
		}
	}

	// Generate the bottom description
	bottomRow := fmt.Sprintf("\t%s", descriptor.description)

	return fmt.Sprintf("%s\n%s", topRow, bottomRow)
}

// FlagSet adds some default commands to handle grpc connections with the server
func (m *Meta) FlagSet(n string) *flag.FlagSet {
	f := flag.NewFlagSet(n, flag.ContinueOnError)
	f.StringVar(&m.addr, "address", "127.0.0.1:9632", "Address of the http api")

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
