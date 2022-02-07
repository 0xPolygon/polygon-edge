package dev

import (
	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/command/server"
	"github.com/spf13/cobra"
)

var (
	params = &devParams{}
)

const (
	dummyBootnode = "/ip4/127.0.0.1/tcp/10001/p2p/16Uiu2HAmJxxH1tScDX2rLGSU9exnuvZKNM9SoK3v315azp68DLPW"
)

type devParams struct {
	// The dev command shadows the genesis and server commands
	genesisCmd *cobra.Command
	serverCmd  *cobra.Command
}

func (p *devParams) initChildCommands() {
	p.genesisCmd = genesis.GetCommand()
	p.serverCmd = server.GetCommand()
}
