package peers

import (
	"fmt"

	"github.com/ryanuber/columnize"
	"github.com/spf13/cobra"

	httpClient "github.com/umbracle/minimal/api/http/client"
	"github.com/umbracle/minimal/command"
)

var peersListCmd = &cobra.Command{
	Use:   "list", // TODO: change to a compiler input string?
	Short: "List...",
	Run:   peersListRun,
	RunE:  peersListRunE,
}

func init() {
	peersCmd.AddCommand(peersListCmd)
}

func peersListRun(cmd *cobra.Command, args []string) {
	command.RunCmd(cmd, args, peersListRunE)
}

func peersListRunE(cmd *cobra.Command, args []string) error {
	format, err := cmd.Flags().GetBool("format")
	if err != nil {
		return err
	}

	// TODO: Finish
	client := httpClient.NewClient("http://127.0.0.1:8600")

	peers, err := client.PeersList()
	if err != nil {
		return fmt.Errorf("Error retrieving peers list: %s", err)
	}

	fmt.Println(
		formatPeers(peers, format),
	)
	return nil
}

func formatPeers(peers []string, format bool) string {
	if len(peers) == 0 {
		return "No peers found"
	}

	rows := make([]string, len(peers)+1)
	rows[0] = "ID"

	for i, d := range peers {
		rowStr := d
		rows[i+1] = rowStr
	}

	return formatList(rows)
}

func formatList(in []string) string {
	columnConf := columnize.DefaultConfig()
	columnConf.Empty = "<none>"
	return columnize.Format(in, columnConf)
}
