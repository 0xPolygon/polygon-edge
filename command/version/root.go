package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// GitCommit is the git commit that was compiled.
	// TODO: Modify makefile to fill the git commit and version
	GitCommit string

	// Version is the main version at the moment.
	Version = "0.1.0"

	// VersionPrerelease is a marker for the version.
	VersionPrerelease = "dev"
)

var versionCmd = &cobra.Command{
	Use:   "version", // TODO: change to a compiler input string?
	Short: "Version printer.",
	Run:   versionRun,
	RunE:  versionRunE,
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func versionRun(cmd *cobra.Command, args []string) {
	runCmd(cmd, args, versionRunE)
}

func versionRunE(cmd *cobra.Command, args []string) error {
	version := Version
	if VersionPrerelease != "" {
		version += fmt.Sprintf("-%s", VersionPrerelease)

		if GitCommit != "" {
			version += fmt.Sprintf(" (%s)", GitCommit)
		}
	}
	fmt.Println(version)
	return nil
}
