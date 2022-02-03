package root

import (
	"github.com/0xPolygon/polygon-edge/command/txpool"
	"github.com/0xPolygon/polygon-edge/command/version"
)

func (rc *RootCommand) registerCommands() {
	rc.baseCmd.AddCommand(version.NewVersionCommand())
	rc.baseCmd.AddCommand(txpool.NewTxPoolCommand())
}
