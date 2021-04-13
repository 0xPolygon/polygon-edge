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

// Commands returns the cli commands
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
		// ---- peers commands ----
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
		// ---- ibft commands ----
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
		// ---- txpool ----
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
		// ---- blockchain commands ----
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

// Meta is a helper utility for the commands
type Meta struct {
	UI   cli.Ui
	addr string
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

func formatList(in []string) string {
	columnConf := columnize.DefaultConfig()
	columnConf.Empty = "<none>"
	return columnize.Format(in, columnConf)
}

func formatKV(in []string) string {
	columnConf := columnize.DefaultConfig()
	columnConf.Empty = "<none>"
	columnConf.Glue = " = "
	return columnize.Format(in, columnConf)
}
