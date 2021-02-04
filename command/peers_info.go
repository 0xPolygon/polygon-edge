package command

// PeersInfo is the PeersInfo to start the sever
type PeersInfo struct {
	Meta
}

// Help implements the cli.PeersInfo interface
func (p *PeersInfo) Help() string {
	return ""
}

// Synopsis implements the cli.PeersInfo interface
func (p *PeersInfo) Synopsis() string {
	return ""
}

// Run implements the cli.PeersInfo interface
func (p *PeersInfo) Run(args []string) int {
	return 0
}
