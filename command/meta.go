package command

import (
	"os"

	"github.com/mitchellh/cli"
	"github.com/mitchellh/colorstring"
	"golang.org/x/crypto/ssh/terminal"
)

type Meta struct {
	Ui cli.Ui
}

func (m *Meta) Colorize() *colorstring.Colorize {
	return &colorstring.Colorize{
		Colors:  colorstring.DefaultColors,
		Disable: !terminal.IsTerminal(int(os.Stdout.Fd())),
		Reset:   true,
	}
}
