package command

import (
	"fmt"
	"strconv"
)

// IbftPropose is the command to query the snapshot
type IbftPropose struct {
	Meta
}

// Help implements the cli.IbftPropose interface
func (p *IbftPropose) Help() string {
	return ""
}

// Synopsis implements the cli.IbftPropose interface
func (p *IbftPropose) Synopsis() string {
	return ""
}

// Run implements the cli.IbftPropose interface
func (p *IbftPropose) Run(args []string) int {
	flags := p.FlagSet("ibft propose")
	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	args = flags.Args()
	if len(args) != 1 {
		p.UI.Error("number expected")
		return 1
	}

	num, err := strconv.Atoi(args[0])
	if err != nil {
		p.UI.Error(fmt.Sprintf("failed to parse snapshot number: %v", err))
		return 1
	}

	conn, err := p.Conn()
	if err != nil {
		p.UI.Error(err.Error())
		return 1
	}
	fmt.Println(conn)
	fmt.Println(num)

	return 0
}
