package command

import (
	"fmt"

	"github.com/umbracle/minimal/version"
)

type VersionCommand struct {
	Meta
}

func (v *VersionCommand) Help() string {
	return ""
}

func (v *VersionCommand) Synopsis() string {
	return ""
}

func (v *VersionCommand) Run(args []string) int {
	v.Meta.Ui.Output(fmt.Sprintf("Minimal %s", version.GetVersion()))
	return 0
}
