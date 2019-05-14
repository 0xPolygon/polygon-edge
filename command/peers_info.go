package command

import (
	"flag"
	"fmt"

	"github.com/mitchellh/mapstructure"
	"github.com/umbracle/minimal/network/common"
)

type PeersInfoCommand struct {
	Meta
}

func (p *PeersInfoCommand) Help() string {
	return ""
}

func (p *PeersInfoCommand) Synopsis() string {
	return ""
}

func (p *PeersInfoCommand) Run(args []string) int {

	flags := flag.NewFlagSet("peers info", flag.ContinueOnError)
	flags.Usage = func() {}

	if err := flags.Parse(args); err != nil {
		return 1
	}

	args = flags.Args()
	if len(args) != 1 {
		p.Ui.Error(fmt.Sprintf("Only one argument expected but %d found", len(args)))
		return 1
	}

	client := p.Meta.Client()

	info, err := client.PeersInfo(args[0])
	if err != nil {
		p.Ui.Error(err.Error())
		return 1
	}
	return p.formatInfo(info)
}

func (p *PeersInfoCommand) formatInfo(info map[string]interface{}) int {
	data := []string{
		fmt.Sprintf("Client|%s", info["client"]),
		fmt.Sprintf("ID|%s", info["id"]),
		fmt.Sprintf("IP|%s", info["ip"]),
	}

	p.Ui.Output(p.Colorize().Color("[bold]Info[reset]"))
	p.Ui.Output(formatKV(data))

	var protos []common.Instance
	if err := mapstructure.Decode(info["protocols"], &protos); err != nil {
		// TODO, handle this error
		return 1
	}

	caps := make([]string, len(protos)+1)
	caps[0] = "Name|Version"

	for indx, c := range protos {
		caps[indx+1] = fmt.Sprintf("%s|%d", c.Protocol.Spec.Name, c.Protocol.Spec.Version)
	}

	p.Ui.Output(p.Colorize().Color("\n[bold]Capabilities[reset]"))
	p.Ui.Output(formatList(caps))

	return 0
}
