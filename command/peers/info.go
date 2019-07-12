package peers

import (
	"fmt"
	"os"

	"github.com/mitchellh/colorstring"
	"github.com/mitchellh/mapstructure"
	"github.com/ryanuber/columnize"
	"github.com/spf13/cobra"
	"github.com/umbracle/minimal/network"
	"golang.org/x/crypto/ssh/terminal"

	httpClient "github.com/umbracle/minimal/api/http/client"
	"github.com/umbracle/minimal/command"
)

var peersInfoCmd = &cobra.Command{
	Use:   "info", // TODO: change to a compiler input string?
	Short: "Info...",
	Run:   peersInfoRun,
	RunE:  peersInfoRunE,
}

func init() {
	peersCmd.AddCommand(peersInfoCmd)
}

func peersInfoRun(cmd *cobra.Command, args []string) {
	command.RunCmd(cmd, args, peersInfoRunE)
}

func peersInfoRunE(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Argument expected")
	}

	client := httpClient.NewClient("http://127.0.0.1:8600")

	info, err := client.PeersInfo(args[0])
	if err != nil {
		return err
	}

	return formatInfo(info)
}

func formatInfo(info map[string]interface{}) error {
	data := []string{
		fmt.Sprintf("Client|%s", info["client"]),
		fmt.Sprintf("ID|%s", info["id"]),
		fmt.Sprintf("IP|%s", info["ip"]),
	}

	color := &colorstring.Colorize{
		Colors:  colorstring.DefaultColors,
		Disable: !terminal.IsTerminal(int(os.Stdout.Fd())),
		Reset:   true,
	}

	// TODO: Should print here or after all the error checking?
	fmt.Println(color.Color("[bold]Info[reset]"))
	fmt.Println(
		formatKV(data),
	)

	var protos []network.ProtocolSpec
	if err := mapstructure.Decode(info["protocols"], &protos); err != nil {
		return err
	}

	caps := make([]string, len(protos)+1)
	caps[0] = "Name|Version"

	for indx, c := range protos {
		caps[indx+1] = fmt.Sprintf("%s|%d", c.Name, c.Version)
	}

	fmt.Println(color.Color("\n[bold]Capabilities[reset]"))
	fmt.Println(formatList(caps))

	return nil
}

func formatKV(in []string) string {
	columnConf := columnize.DefaultConfig()
	columnConf.Empty = "<none>"
	columnConf.Glue = " = "
	return columnize.Format(in, columnConf)
}
