package version

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/version"
)

// VersionCommand is the command to show the version of the agent
type VersionCommand struct {
	helper.Base
	Formatter *helper.FormatterFlag
}

// DefineFlags defines the command flags
func (c *VersionCommand) DefineFlags() {
	c.Base.DefineFlags(c.Formatter)
}

// GetHelperText returns a simple description of the command
func (c *VersionCommand) GetHelperText() string {
	return "Returns the current Polygon Edge version"
}

func (c *VersionCommand) GetBaseCommand() string {
	return "version"
}

// Help implements the cli.Command interface
func (c *VersionCommand) Help() string {
	c.DefineFlags()

	return helper.GenerateHelp(c.Synopsis(), helper.GenerateUsage(c.GetBaseCommand(), c.FlagMap), c.FlagMap)
}

// Synopsis implements the cli.Command interface
func (c *VersionCommand) Synopsis() string {
	return c.GetHelperText()
}

// Run implements the cli.Command interface
func (c *VersionCommand) Run(args []string) int {
	flags := c.Base.NewFlagSet(c.GetBaseCommand(), c.Formatter)
	if err := flags.Parse(args); err != nil {
		c.Formatter.OutputError(err)

		return 1
	}

	c.Formatter.OutputResult(&VersionResult{
		Version: version.Version,
	})

	return 0
}

type VersionResult struct {
	Version string `json:"version"`
}

func (r *VersionResult) Output() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[POLYGON EDGE VERSION]\n")
	buffer.WriteString(r.Version)
	buffer.WriteString("\n")

	buffer.WriteString("\n[LICENSE]\n\n")
	buffer.WriteString(version.License)
	buffer.WriteString("\n")

	buffer.WriteString("\n[DEPENDENCY LICENSES]\n\n")

	for _, l := range version.BsdLicenses {
		buffer.WriteString(fmt.Sprintf(
			"   This product bundles %s %s,\n"+
				"   which is available under a \"%s\" license.\n"+
				"   For details, see %s.\n\n",
			l.Name,
			l.Version,
			l.Type,
			l.Path,
		))
	}

	return buffer.String()
}
