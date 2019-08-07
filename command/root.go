package command

import (
	logger "github.com/hashicorp/go-hclog"
	"github.com/spf13/cobra"
	"github.com/umbracle/minimal/command/debug"
)

// RegisterCmd adds the parsed cmd as a sub-command of the root cmd.
func RegisterCmd(cmd *cobra.Command) {
	rootCmd.AddCommand(cmd)
}

// CobraFunc is the generic cobra function to execute in a cli
type CobraFunc func(cmd *cobra.Command, args []string) error

var rootCmd = &cobra.Command{
	Use:   "minimal", // TODO: change to a compiler input string?
	Short: "Minimal is a lightweight client for the Ethereum blockchain.",
	Long:  "Minimal is ...", // TODO: Add long description (or not)
	Run:   rootRun,
	RunE:  rootRunE,
}

func init() {
	rootCmd.AddCommand(debug.DebugCmd)
}

// Run runs the root command which initialises the program.
var Run = rootCmd.Execute // TODO: Don't know if it's a good idea

func rootRun(cmd *cobra.Command, args []string) {
	RunCmd(cmd, args, rootRunE)
}

// There is no root command
func rootRunE(cmd *cobra.Command, args []string) error {
	return cmd.Usage()
}

// RunCmd is a helper to run a generic command
//
// It's intended to be used by the sub-commands
func RunCmd(cmd *cobra.Command, args []string, fn CobraFunc) {
	if fn == nil {
		panic("CobraFunc cannot be nil")
	}

	if err := fn(cmd, args); err != nil {
		logger.Default().Error("error running", cmd.Use, err)
	}
}
