package bridge

import (
	"github.com/spf13/cobra"

	depositERC1155 "github.com/0xPolygon/polygon-edge/command/bridge/deposit/erc1155"
	depositERC20 "github.com/0xPolygon/polygon-edge/command/bridge/deposit/erc20"
	depositERC721 "github.com/0xPolygon/polygon-edge/command/bridge/deposit/erc721"
	"github.com/0xPolygon/polygon-edge/command/bridge/exit"
	"github.com/0xPolygon/polygon-edge/command/bridge/mint"
	withdrawERC1155 "github.com/0xPolygon/polygon-edge/command/bridge/withdraw/erc1155"
	withdrawERC20 "github.com/0xPolygon/polygon-edge/command/bridge/withdraw/erc20"
	withdrawERC721 "github.com/0xPolygon/polygon-edge/command/bridge/withdraw/erc721"
)

// GetCommand creates "bridge" helper command
func GetCommand() *cobra.Command {
	bridgeCmd := &cobra.Command{
		Use:   "bridge",
		Short: "Top level bridge command.",
	}

	registerSubcommands(bridgeCmd)

	return bridgeCmd
}

func registerSubcommands(baseCmd *cobra.Command) {
	baseCmd.AddCommand(
		// bridge deposit-erc20
		depositERC20.GetCommand(),
		// bridge deposit-erc721
		depositERC721.GetCommand(),
		// bridge deposit-erc1155
		depositERC1155.GetCommand(),
		// bridge withdraw-erc20
		withdrawERC20.GetCommand(),
		// bridge withdraw-erc721
		withdrawERC721.GetCommand(),
		// bridge withdraw-erc1155
		withdrawERC1155.GetCommand(),
		// bridge exit
		exit.GetCommand(),
		// bridge mint erc-20
		mint.GetCommand(),
	)
}
