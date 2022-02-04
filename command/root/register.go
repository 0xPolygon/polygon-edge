package root

import (
	"github.com/0xPolygon/polygon-edge/command/status"
	"github.com/0xPolygon/polygon-edge/command/txpool"
	"github.com/0xPolygon/polygon-edge/command/version"
)

func (rc *RootCommand) registerCommands() {
	rc.baseCmd.AddCommand(version.GetCommand())
	rc.baseCmd.AddCommand(txpool.GetCommand())
	rc.baseCmd.AddCommand(status.GetCommand())
}
