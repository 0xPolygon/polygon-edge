package command

import (
	"os"

	"github.com/mitchellh/cli"
	"github.com/umbracle/minimal/command/agent"
)

// Commands returns the mapping of CLI commands for Minimal
func Commands() map[string]cli.CommandFactory {
	ui := &cli.BasicUi{
		Reader:      os.Stdin,
		Writer:      os.Stdout,
		ErrorWriter: os.Stderr,
	}

	meta := Meta{
		Ui: ui,
	}

	return map[string]cli.CommandFactory{
		"agent": func() (cli.Command, error) {
			return &agent.AgentCommand{
				Ui: ui,
			}, nil
		},
		"genesis": func() (cli.Command, error) {
			return &GenesisCommand{
				Meta: meta,
			}, nil
		},
		"peers": func() (cli.Command, error) {
			return &PeersCommand{
				Meta: meta,
			}, nil
		},
		"peers list": func() (cli.Command, error) {
			return &PeersListCommand{
				Meta: meta,
			}, nil
		},
		"peers info": func() (cli.Command, error) {
			return &PeersInfoCommand{
				Meta: meta,
			}, nil
		},
	}
}
