package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/0xPolygon/minimal/command"
	"github.com/0xPolygon/minimal/version"
)

var versionCmd = &cobra.Command{
	Use:   "version", // TODO: change to a compiler input string?
	Short: "Version printer.",
	Run:   versionRun,
	RunE:  versionRunE,
}

func init() {
	command.RegisterCmd(versionCmd)
}

func versionRun(cmd *cobra.Command, args []string) {
	command.RunCmd(cmd, args, versionRunE)
}

func versionRunE(cmd *cobra.Command, args []string) error {
	fmt.Println(
		version.GetVersion(),
	)
	return nil
}
