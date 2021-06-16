package command

import (
	"flag"
	"fmt"
	"os"

	"github.com/0xPolygon/minimal/command/helper"
	"github.com/0xPolygon/minimal/command/server"
	"github.com/0xPolygon/minimal/minimal"
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

	// Grab a reference to the commands
	serverCmd := server.Command{UI: ui}
	genesisCmd := GenesisCommand{UI: ui}
	monitorCmd := MonitorCommand{Meta: meta}
	statusCmd := StatusCommand{Meta: meta}
	versionCmd := VersionCommand{UI: ui}

	ibftCmd := IbftCommand{}
	ibftCandidatesCmd := IbftCandidates{Meta: meta}
	ibftInitCmd := IbftInit{Meta: meta}
	ibftProposeCmd := IbftPropose{Meta: meta}
	ibftSnapshotCmd := IbftSnapshot{Meta: meta}
	ibftStatusCmd := IbftStatus{Meta: meta}

	peersCmd := PeersCommand{}
	peersAddCmd := PeersAdd{Meta: meta}
	peersListCmd := PeersList{Meta: meta}
	peersStatusCmd := PeersStatus{Meta: meta}

	txPoolCmd := TxPoolCommand{}
	txPoolAddCmd := TxPoolAdd{Meta: meta}
	txPoolStatusCmd := TxPoolStatus{Meta: meta}

	return map[string]cli.CommandFactory{

		// GENERIC SDK COMMANDS //

		serverCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &serverCmd, nil
		},
		// TODO The task to implement the dev command should start here
		//"dev": func() (cli.Command, error) {
		//	return &DevCommand{
		//		UI: ui,
		//	}, nil
		//},
		genesisCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &genesisCmd, nil
		},

		// PEER COMMANDS //

		peersCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &peersCmd, nil
		},
		peersAddCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &peersAddCmd, nil
		},
		peersStatusCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &peersStatusCmd, nil
		},
		peersListCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &peersListCmd, nil
		},

		// IBFT COMMANDS //

		ibftCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &ibftCmd, nil
		},

		ibftInitCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &ibftInitCmd, nil
		},
		ibftSnapshotCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &ibftSnapshotCmd, nil
		},
		ibftCandidatesCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &ibftCandidatesCmd, nil
		},
		ibftProposeCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &ibftProposeCmd, nil
		},
		ibftStatusCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &ibftStatusCmd, nil
		},

		// TXPOOL COMMANDS //

		txPoolCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &txPoolCmd, nil
		},
		txPoolAddCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &txPoolAddCmd, nil
		},
		txPoolStatusCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &txPoolStatusCmd, nil
		},

		// BLOCKCHAIN COMMANDS //

		statusCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &statusCmd, nil
		},
		monitorCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &monitorCmd, nil
		},
		versionCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &versionCmd, nil
		},
	}
}

type HelpGenerator interface {
	DefineFlags()
}

// Meta is a helper utility for the commands
type Meta struct {
	UI   cli.Ui
	addr string

	flagMap        map[string]helper.FlagDescriptor
	hasGlobalFlags bool
}

// DefineFlags sets global flags used by several commands
func (m *Meta) DefineFlags() {
	m.hasGlobalFlags = true
	m.flagMap = make(map[string]helper.FlagDescriptor)

	m.flagMap["grpc-address"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Address of the gRPC API. Default: %s:%d", "127.0.0.1", minimal.DefaultGRPCPort),
		Arguments: []string{
			"GRPC_ADDRESS",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
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
