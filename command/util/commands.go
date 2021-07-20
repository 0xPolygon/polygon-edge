package util

import (
	"os"

	"github.com/0xPolygon/minimal/command/dev"
	"github.com/0xPolygon/minimal/command/genesis"
	"github.com/0xPolygon/minimal/command/helper"
	"github.com/0xPolygon/minimal/command/ibft"
	"github.com/0xPolygon/minimal/command/monitor"
	"github.com/0xPolygon/minimal/command/peers"
	"github.com/0xPolygon/minimal/command/server"
	"github.com/0xPolygon/minimal/command/status"
	"github.com/0xPolygon/minimal/command/txpool"
	"github.com/0xPolygon/minimal/command/version"
	"github.com/mitchellh/cli"
)

// Commands returns a mapping of all available commands
func Commands() map[string]cli.CommandFactory {
	ui := &cli.BasicUi{
		Reader:      os.Stdin,
		Writer:      os.Stdout,
		ErrorWriter: os.Stderr,
	}

	meta := helper.Meta{
		UI: ui,
	}

	// Grab a reference to the commands
	serverCmd := server.ServerCommand{UI: ui}
	devCmd := dev.DevCommand{UI: ui}
	genesisCmd := genesis.GenesisCommand{UI: ui}
	monitorCmd := monitor.MonitorCommand{Meta: meta}
	statusCmd := status.StatusCommand{Meta: meta}
	versionCmd := version.VersionCommand{UI: ui}

	ibftCmd := ibft.IbftCommand{}
	ibftCandidatesCmd := ibft.IbftCandidates{Meta: meta}
	ibftInitCmd := ibft.IbftInit{Meta: meta}
	ibftProposeCmd := ibft.IbftPropose{Meta: meta}
	ibftSnapshotCmd := ibft.IbftSnapshot{Meta: meta}
	ibftStatusCmd := ibft.IbftStatus{Meta: meta}

	peersCmd := peers.PeersCommand{}
	peersAddCmd := peers.PeersAdd{Meta: meta}
	peersListCmd := peers.PeersList{Meta: meta}
	peersStatusCmd := peers.PeersStatus{Meta: meta}

	txPoolCmd := txpool.TxPoolCommand{}
	txPoolAddCmd := txpool.TxPoolAdd{Meta: meta}
	txPoolStatusCmd := txpool.TxPoolStatus{Meta: meta}

	return map[string]cli.CommandFactory{

		// GENERIC SDK COMMANDS //

		serverCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &serverCmd, nil
		},
		devCmd.GetBaseCommand(): func() (cli.Command, error) {
			return &devCmd, nil
		},
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
