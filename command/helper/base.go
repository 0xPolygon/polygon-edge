package helper

import (
	"flag"

	"github.com/mitchellh/cli"
)

type FlagDefiner interface {
	DefineFlags(map[string]FlagDescriptor)
}

type FlagSetter interface {
	FlagSet(*flag.FlagSet)
}

type CommandResult interface {
	// Return result for stdout
	Output() string
}

// Base has common fields for each command
type Base struct {
	UI      cli.Ui
	FlagMap map[string]FlagDescriptor
}

// DefineFlags initializes and defines the common command flags
func (c *Base) DefineFlags(ds ...FlagDefiner) {
	if c.FlagMap == nil {
		// Flag map not initialized
		c.FlagMap = make(map[string]FlagDescriptor)
	}
	for _, d := range ds {
		d.DefineFlags(c.FlagMap)
	}
}

// FlagSet initializes Flag Set
func (m *Base) NewFlagSet(n string, ss ...FlagSetter) *flag.FlagSet {
	flag := flag.NewFlagSet(n, flag.ContinueOnError)
	for _, s := range ss {
		s.FlagSet(flag)
	}
	return flag
}
