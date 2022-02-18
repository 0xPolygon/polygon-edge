package main

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/0xPolygon/polygon-edge/command/util"
	"github.com/0xPolygon/polygon-edge/licenses"
	"github.com/mitchellh/cli"
)

var (
	//go:embed LICENSE
	license string
)

func main() {
	licenses.SetLicense(license)

	os.Exit(Run(os.Args[1:]))
}

// Run starts the cli
func Run(args []string) int {
	commands := util.Commands()

	cli := &cli.CLI{
		Name:     "polygon",
		Args:     args,
		Commands: commands,
	}

	exitCode, err := cli.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error executing CLI: %s\n", err.Error())

		return 1
	}

	return exitCode
}
