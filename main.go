package main

import (
	"fmt"
	"os"

	"github.com/0xPolygon/minimal/command"
	"github.com/mitchellh/cli"
)

func main() {
	os.Exit(Run(os.Args[1:]))
}

// Run starts the cli
func Run(args []string) int {
	commands := command.Commands()

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

/*
import (
	logger "github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/minimal/command"
	_ "github.com/0xPolygon/minimal/command/agent"
	_ "github.com/0xPolygon/minimal/command/debug"
	_ "github.com/0xPolygon/minimal/command/dev"
	_ "github.com/0xPolygon/minimal/command/genesis"
	_ "github.com/0xPolygon/minimal/command/ibft-genesis"
	_ "github.com/0xPolygon/minimal/command/peers"
	_ "github.com/0xPolygon/minimal/command/version"
)

func main() {
	// TODO: Change time format for the logger?
	if err := command.Run(); err != nil {
		logger.Default().Error(err.Error())
	}
}
*/
