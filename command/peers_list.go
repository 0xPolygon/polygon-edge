package command

// PeersList is the PeersList to start the sever
type PeersList struct {
	Meta
}

// Help implements the cli.PeersList interface
func (p *PeersList) Help() string {
	return ""
}

// Synopsis implements the cli.PeersList interface
func (p *PeersList) Synopsis() string {
	return ""
}

// Run implements the cli.PeersList interface
func (p *PeersList) Run(args []string) int {
	return 0
}
