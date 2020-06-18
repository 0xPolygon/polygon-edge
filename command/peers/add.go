package peers

import (
	"fmt"

	"github.com/spf13/cobra"

	httpClient "github.com/0xPolygon/minimal/api/http/client"
	"github.com/0xPolygon/minimal/command"
)

var peersAddCmd = &cobra.Command{
	Use:   "add", // TODO: change to a compiler input string?
	Short: "Add...",
	Run:   peersAdd,
	RunE:  peersAddE,
}

func init() {
	peersCmd.AddCommand(peersAddCmd)
}

func peersAdd(cmd *cobra.Command, args []string) {
	command.RunCmd(cmd, args, peersAddE)
}

func peersAddE(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("expected one argument")
	}
	client := httpClient.NewClient("http://127.0.0.1:8600")
	if err := client.PeersAdd(args[0]); err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("Connected!")
	}
	return nil
}
