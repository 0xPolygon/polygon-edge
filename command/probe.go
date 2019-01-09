package command

import (
	"flag"
	"fmt"

	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/protocol"
	"github.com/umbracle/minimal/protocol/ethereum"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/umbracle/minimal/network/discover"
	"github.com/umbracle/minimal/network/rlpx"
)

type ProbeCommand struct {
	Meta
}

func (p *ProbeCommand) Help() string {
	return ""
}

func (p *ProbeCommand) Synopsis() string {
	return ""
}

func (p *ProbeCommand) Run(args []string) int {
	f := flag.NewFlagSet("probe", flag.ContinueOnError)

	if err := f.Parse(args); err != nil {
		p.Ui.Error(fmt.Sprintf("Failed to parse args: %v", err))
		return 1
	}

	args = f.Args()
	if len(args) != 1 {
		p.Ui.Error(fmt.Sprintf("Expected one argument"))
		return 1
	}

	enode := args[0]
	prv, _ := crypto.GenerateKey()

	// later on use a protocol specified by the user (either parity or ethereum)
	localInfo := &rlpx.Info{
		Version:    5,
		Name:       "minimal-probe",
		ListenPort: 30303,
		Caps:       rlpx.Capabilities{&rlpx.Cap{Name: "eth", Version: 63}},
		ID:         discover.PubkeyToNodeID(&prv.PublicKey),
	}

	s, err := rlpx.DialEnode("tcp", enode, &rlpx.Config{Prv: prv, Info: localInfo})
	if err != nil {
		p.Ui.Error(fmt.Sprintf("Failed to connect: %v", err))
		return 1
	}

	info := s.RemoteInfo()

	proto := protocol.ETH63
	ss := s.OpenStream(uint(rlpx.BaseProtocolLength), uint(proto.Length))

	// send a dummy status
	e := ethereum.NewEthereumProtocol(ss, nil, nil, nil)

	remoteStatus, err := e.ReadStatus()
	if err != nil {
		p.Ui.Error(fmt.Sprintf("Failed to read status: %v", err))
		return 1
	}
	return p.formatInfo(info, remoteStatus)
}

func (p *ProbeCommand) formatInfo(info *rlpx.Info, status *ethereum.Status) int {
	data := []string{
		fmt.Sprintf("Name|%s", info.Name),
		fmt.Sprintf("Version|%d", info.Version),
	}

	p.Ui.Output(p.Colorize().Color("[bold]Info[reset]"))
	p.Ui.Output(formatKV(data))

	caps := make([]string, len(info.Caps)+1)
	caps[0] = "Name|Version"

	for indx, c := range info.Caps {
		caps[indx+1] = fmt.Sprintf("%s|%d", c.Name, c.Version)
	}

	p.Ui.Output(p.Colorize().Color("\n[bold]Capabilities[reset]"))
	p.Ui.Output(formatList(caps))

	// Ethereum data
	data = []string{
		fmt.Sprintf("Genesis|%s", status.GenesisBlock.String()),
		fmt.Sprintf("Current|%s", status.CurrentBlock.String()),
		fmt.Sprintf("Difficulty|%s", status.TD.String()),
		fmt.Sprintf("Network|%s", chain.ResolveNetworkID(uint(status.NetworkID))),
		fmt.Sprintf("Protocol|%d", status.ProtocolVersion),
	}

	p.Ui.Output(p.Colorize().Color("\n[bold]Ethereum[reset]"))
	p.Ui.Output(formatKV(data))
	return 0
}
