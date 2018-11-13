package syncer

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/network"
	"github.com/umbracle/minimal/protocol/ethereum"
)

type callback struct {
	id  uint32
	ack chan bool
}

// Peer is a network connection in the syncer
type Peer struct {
	peer    *network.Peer
	eth     *ethereum.Ethereum
	closeCh chan struct{}
	syncer  *Syncer

	workerPool chan chan Job

	// -- pending operations (TODO. make an struct of this)
	pendingBlocks     map[uint64]*callback
	pendingBlocksLock sync.Mutex

	timer *time.Timer
}

// NewPeer creates a new peer
func NewPeer(peer *network.Peer, workerPool chan chan Job, s *Syncer) *Peer {
	eth := peer.GetProtocol("eth", 63).(*ethereum.Ethereum)

	p := &Peer{
		peer:              peer,
		eth:               eth,
		closeCh:           make(chan struct{}),
		workerPool:        workerPool,
		syncer:            s,
		pendingBlocks:     make(map[uint64]*callback),
		pendingBlocksLock: sync.Mutex{},
	}

	eth.SetDownloader(p)
	return p
}

// Close closes the connection
func (p *Peer) Close() {
	close(p.closeCh)
}

// put another timer as in the handler

func (p *Peer) requestTask(id string) {
	jobChannel := make(chan Job, 10)

	for {
		p.workerPool <- jobChannel

		select {
		case job := <-jobChannel:

			fmt.Printf("SYNC (%s) (%s): %d\n", p.peer.PrettyString(), id, job.block)

			res := make(chan bool, 1)
			p.requestHeader(job, res)

			// handle result
			<-res

		case <-p.closeCh:
			return
		}
	}
}

func (p *Peer) requestHeader(job Job, ack chan bool) error {
	if err := p.eth.RequestHeadersByNumber(job.block, 100, 0, false); err != nil {
		return err
	}

	p.pendingBlocksLock.Lock()
	p.pendingBlocks[job.block] = &callback{job.id, ack}
	p.pendingBlocksLock.Unlock()

	p.timer = time.AfterFunc(5*time.Second, func() {
		if _, ok := p.pendingBlocks[job.block]; !ok {
			return
		}

		p.pendingBlocksLock.Lock()
		delete(p.pendingBlocks, job.block)
		p.pendingBlocksLock.Unlock()

		ack <- false
		p.syncer.result(job.id, nil, fmt.Errorf("timeout"))
	})

	return nil
}

func (p *Peer) run() {
	for i := 0; i < 5; i++ {
		go p.requestTask(strconv.Itoa(i))
	}
}

// -- downloader

// Headers receives the headers
func (p *Peer) Headers(headers []*types.Header) {

	if len(headers) == 0 {
		return
	}

	origin := headers[0].Number.Uint64()

	// check if its in pending blocks
	callback, ok := p.pendingBlocks[origin]
	if !ok {
		return
	}

	// delete
	p.pendingBlocksLock.Lock()
	delete(p.pendingBlocks, origin)
	p.pendingBlocksLock.Unlock()

	// let him know its over
	select {
	case callback.ack <- true:
	default:
	}

	p.syncer.result(callback.id, headers, nil)
}
