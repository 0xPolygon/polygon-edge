package command

// PeersAdd is the PeersAdd to start the sever
type PeersAdd struct {
	Meta
}

// Help implements the cli.PeersAdd interface
func (p *PeersAdd) Help() string {
	return ""
}

// Synopsis implements the cli.PeersAdd interface
func (p *PeersAdd) Synopsis() string {
	return ""
}

// Run implements the cli.PeersAdd interface
func (p *PeersAdd) Run(args []string) int {
	return 0
}
