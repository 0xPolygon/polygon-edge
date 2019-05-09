package command

type PeersCommand struct {
	Meta
}

func (p *PeersCommand) Help() string {
	return ""
}

func (p *PeersCommand) Synopsis() string {
	return ""
}

func (p *PeersCommand) Run(args []string) int {
	return 1
}
