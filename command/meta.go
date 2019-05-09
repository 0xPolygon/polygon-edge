package command

import (
	"os"

	"github.com/mitchellh/cli"
	"github.com/mitchellh/colorstring"
	httpClient "github.com/umbracle/minimal/api/http/client"
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

func (m *Meta) Client() *httpClient.Client {
	// TODO: read address from config file
	return httpClient.NewClient("http://127.0.0.1:8600")
}
