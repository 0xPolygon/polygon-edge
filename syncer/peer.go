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

// AckMessage is the ack message
type AckMessage struct {
	Complete bool
	Result   interface{}
}

type callback struct {
	id  uint32
	ack chan AckMessage
}

type dispatcher interface {
	Result(uint32, []*types.Header, error)
}

// Peer is a network connection in the syncer
type Peer struct {
	peer *network.Peer
	eth  *ethereum.Ethereum

	syncer *Syncer

	workerPool chan chan Job
	closeCh    chan struct{}
	once       sync.Once

	// -- pending operations (TODO. make an struct of this)
	pendingBlocks     map[uint64]*callback
	pendingBlocksLock sync.Mutex

	timer *time.Timer
}

// NewPeer creates a new peer
func NewPeer(peer *network.Peer, workerPool chan chan Job, syncer *Syncer) *Peer {
	eth := peer.GetProtocol("eth", 63).(*ethereum.Ethereum)

	p := &Peer{
		peer:              peer,
		eth:               eth,
		closeCh:           make(chan struct{}),
		workerPool:        workerPool,
		syncer:            syncer,
		pendingBlocks:     make(map[uint64]*callback),
		pendingBlocksLock: sync.Mutex{},
	}

	eth.SetDownloader(p)
	return p
}

// put another timer as in the handler

func (p *Peer) stopTasks() {
	p.once.Do(func() {
		fmt.Printf("STOPPING TASKS (%s)\n", p.peer.PrettyString())
		close(p.closeCh)
	})
}

func (p *Peer) requestTask(id string) {
	jobChannel := make(chan Job, 10)

	for {
		p.workerPool <- jobChannel

		select {
		case job := <-jobChannel:

			fmt.Printf("SYNC (%s) (%s): %d\n", p.peer.PrettyString(), id, job.block)

			headers, err := p.requestJob(job)
			if err != nil {
				// check if the connection is still open
				if p.peer.Connected == false {
					// close all the download tasks
					p.stopTasks()
				}
			}
			p.syncer.Result(job.id, headers, err)

		case <-p.closeCh:
			return
		}
	}
}

func (p *Peer) requestJob(j Job) ([]*types.Header, error) {
	ack := make(chan AckMessage, 1)
	if err := p.requestHeader(j, ack); err != nil {
		return nil, err
	}

	// wait for ack
	resp := <-ack
	if !resp.Complete {
		return nil, fmt.Errorf("timeout")
	}

	return resp.Result.([]*types.Header), nil
}

func (p *Peer) requestHeader(job Job, ack chan AckMessage) error {
	if err := p.eth.RequestHeadersByNumber(job.block, 100, 0, false); err != nil {
		return err
	}

	p.pendingBlocksLock.Lock()
	p.pendingBlocks[job.block] = &callback{job.id, ack}
	p.pendingBlocksLock.Unlock()

	p.timer = time.AfterFunc(5*time.Second, func() {
		p.pendingBlocksLock.Lock()
		if _, ok := p.pendingBlocks[job.block]; !ok {
			p.pendingBlocksLock.Unlock()
			return
		}

		delete(p.pendingBlocks, job.block)
		p.pendingBlocksLock.Unlock()

		select {
		case ack <- AckMessage{false, nil}:
		default:
		}
	})

	return nil
}

func (p *Peer) run(maxRequests int) {
	for i := 0; i < maxRequests; i++ {
		go p.requestTask(strconv.Itoa(i))
	}
}

// -- downloader --

// Headers receives the headers
func (p *Peer) Headers(headers []*types.Header) {
	if len(headers) == 0 {
		// The peer did not have the headers of the query,
		// we cannot know to which pending query it belongs
		return
	}

	origin := headers[0].Number.Uint64()

	// check if its in pending blocks
	p.pendingBlocksLock.Lock()
	callback, ok := p.pendingBlocks[origin]
	if !ok {
		p.pendingBlocksLock.Unlock()
		return
	}

	// delete
	delete(p.pendingBlocks, origin)
	p.pendingBlocksLock.Unlock()

	// let him know its over
	select {
	case callback.ack <- AckMessage{Complete: true, Result: headers}:
	default:
	}
}
