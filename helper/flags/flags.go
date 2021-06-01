package flags

import (
	"github.com/multiformats/go-multiaddr"
	"strings"
)

func formatArrayForOutput(array []string) string {
	return "(" + strings.Join(array, ",") + ")"
}

type ArrayFlags []string

func (i *ArrayFlags) String() string {
	return formatArrayForOutput(*i)
}

func (i *ArrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

type BootnodeFlags []string

func (i *BootnodeFlags) String() string {
	return formatArrayForOutput(*i)
}

func (i *BootnodeFlags) Set(value string) error {
	if _, err := multiaddr.NewMultiaddr(value); err != nil {
		return err
	}
	*i = append(*i, value)
	return nil
}
