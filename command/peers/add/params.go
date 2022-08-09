package add

import (
	"context"
	"errors"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/server/proto"
)

var (
	params = &addParams{
		addedPeers: make([]string, 0),
		addErrors:  make([]string, 0),
	}
)

var (
	errInvalidAddresses = errors.New("at least 1 peer address is required")
)

const (
	addrFlag = "addr"
)

type addParams struct {
	peerAddresses []string

	systemClient proto.SystemClient

	addedPeers []string
	addErrors  []string
}

func (p *addParams) getRequiredFlags() []string {
	return []string{
		addrFlag,
	}
}

func (p *addParams) validateFlags() error {
	if len(p.peerAddresses) < 1 {
		return errInvalidAddresses
	}

	return nil
}

func (p *addParams) initSystemClient(grpcAddress string) error {
	systemClient, err := helper.GetSystemClientConnection(grpcAddress)
	if err != nil {
		return err
	}

	p.systemClient = systemClient

	return nil
}

func (p *addParams) addPeers() {
	for _, address := range p.peerAddresses {
		if addErr := p.addPeer(address); addErr != nil {
			p.addErrors = append(p.addErrors, addErr.Error())

			continue
		}

		p.addedPeers = append(p.addedPeers, address)
	}
}

func (p *addParams) addPeer(peerAddress string) error {
	if _, err := p.systemClient.PeersAdd(
		context.Background(),
		&proto.PeersAddRequest{
			Id: peerAddress,
		},
	); err != nil {
		return err
	}

	return nil
}

func (p *addParams) getResult() command.CommandResult {
	return &PeersAddResult{
		NumRequested: len(p.peerAddresses),
		NumAdded:     len(p.addedPeers),
		Peers:        p.addedPeers,
		Errors:       p.addErrors,
	}
}
