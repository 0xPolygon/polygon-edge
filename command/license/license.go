package license

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/params"
)

// LicenseCommand is the command to show the version of the agent
type LicenseCommand struct {
	helper.Base
}

// DefineFlags defines the command flags
func (c *LicenseCommand) DefineFlags() {
	c.Base.DefineFlags()
}

// GetHelperText returns a simple description of the command
func (c *LicenseCommand) GetHelperText() string {
	return "Returns Polygon Edge license and dependency attributions"
}

func (c *LicenseCommand) GetBaseCommand() string {
	return "license"
}

// Help implements the cli.Command interface
func (c *LicenseCommand) Help() string {
	c.DefineFlags()

	return helper.GenerateHelp(c.Synopsis(), helper.GenerateUsage(c.GetBaseCommand(), c.FlagMap), c.FlagMap)
}

// Synopsis implements the cli.Command interface
func (c *LicenseCommand) Synopsis() string {
	return c.GetHelperText()
}

// Run implements the cli.Command interface
func (c *LicenseCommand) Run(args []string) int {
	var buffer bytes.Buffer

	buffer.WriteString("\n[LICENSE]\n\n")
	buffer.WriteString(params.License)

	buffer.WriteString("\n[DEPENDENCY ATTRIBUTIONS]\n\n")

	for idx, l := range params.BsdLicenses {
		// put a blank line between attributions
		if idx != 0 {
			buffer.WriteString("\n")
		}

		name := l.Name
		if l.Version != nil {
			name += " " + *l.Version
		}

		buffer.WriteString(fmt.Sprintf(
			"   This product bundles %s,\n"+
				"   which is available under a \"%s\" license.\n"+
				"   For details, see %s.\n",
			name,
			l.Type,
			l.Path,
		))
	}

	c.UI.Info(buffer.String())

	return 0
}
