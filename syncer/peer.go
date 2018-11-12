package syncer

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/umbracle/minimal/network"
	"github.com/umbracle/minimal/protocol/ethereum"
)

// Peer is a network connection in the syncer
type Peer struct {
	peer    *network.Peer
	eth     *ethereum.Ethereum
	closeCh chan struct{}
	s       *Syncer

	workerPool chan chan Job
	jobChannel chan Job
}

// NewPeer creates a new peer
func NewPeer(peer *network.Peer, workerPool chan chan Job, s *Syncer) *Peer {
	return &Peer{
		peer:       peer,
		eth:        peer.GetProtocol("eth", 63).(*ethereum.Ethereum),
		closeCh:    make(chan struct{}),
		workerPool: workerPool,
		jobChannel: make(chan Job),
		s:          s,
	}
}

// Close closes the connection
func (p *Peer) Close() {
	close(p.closeCh)
}

func (p *Peer) run() {
	for {
		p.workerPool <- p.jobChannel

		select {
		case job := <-p.jobChannel:
			headers, err := p.request(job.block)
			p.s.result(job.id, headers, err)
			time.Sleep(1 * time.Second)
		case <-p.closeCh:
			return
		}
	}
}

func (p *Peer) request(block uint64) ([]*types.Header, error) {
	ack := make(chan network.AckMessage, 1)
	p.eth.Conn().SetHandler(ethereum.BlockHeadersMsg, ack, 5*time.Second)

	if err := p.eth.RequestHeadersByNumber(block, 100, 0, false); err != nil {
		return nil, err
	}

	resp := <-ack
	if resp.Complete {
		var headers []*types.Header
		if err := rlp.DecodeBytes(resp.Payload, &headers); err != nil {
			return nil, err
		}
		return headers, nil
	}

	return nil, fmt.Errorf("timeout")
}
