package ethereum

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/armon/go-metrics"
)

type PeerConnection struct {
	quota int
	sched *Backend
	conn  *Ethereum
	id    string
	// peer   *network.Peer

	rateLock sync.Mutex
	rate     int
	counter  int

	stopFn  context.CancelFunc
	running bool

	enabled bool
}

func (p *PeerConnection) requestBandwidth(bytes int) {
	//fmt.Printf("Peer %s request bandwidth: %d\n", p.peerID, bytes)
	//req, rest := p.sched.requestBandwidth(bytes)
	//fmt.Printf("Bandwidth requested %d, rest %d\n", req, rest)
}

func (p *PeerConnection) run() {
	for {
		time.Sleep(1 * time.Second)
		p.rateLock.Lock()

		sample := p.counter

		fmt.Printf("Last p rate %d\n", p.rate)
		fmt.Printf("Count: %d\n", sample)

		p.rate = p.rate*4/5 + sample/5

		// emit keys
		metrics.SetGaugeWithLabels([]string{"minimal", "protocol", "ethereum63", "rate_2"}, float32(p.rate), []metrics.Label{{Name: "id", Value: p.id}})
		metrics.SetGaugeWithLabels([]string{"minimal", "protocol", "ethereum63", "rate"}, float32(sample), []metrics.Label{{Name: "id", Value: p.id}})
		// bytes/second

		fmt.Printf("==> Rate (%s): %d\n", p.id, p.rate)
		p.counter = 0
		p.rateLock.Unlock()
	}
}

func (p *PeerConnection) updateRate(bytes int) {
	fmt.Printf("Update rate (%s): %d\n", p.id, bytes)

	p.rateLock.Lock()
	defer p.rateLock.Unlock()

	p.counter += bytes
}

// Reset resets the peer connection and stops all the current connections
func (p *PeerConnection) Reset() {
	p.running = false // TODO, protect with lock
	if p.stopFn != nil {
		p.stopFn()
	}

	// Start again all the actions, TODO, dont start action() again.
	if p.enabled {
		p.Run()
	}
}

// Run runs the action
func (p *PeerConnection) Run() {

	// go p.run()

	ctx, cancel := context.WithCancel(context.Background())
	p.stopFn = cancel
	p.running = true

	go p.action(ctx)
	// go p.action()

	// go p.action()
}

func (p *PeerConnection) action(ctx context.Context) {
	fmt.Println("############## PEER CONNECTION HAS STARTED #################")

	for {
	CHECK:
		i := p.sched.Dequeue()
		if i == nil {
			// No jobs waiting, sleep and check again
			time.Sleep(5 * time.Second)
			goto CHECK
		}

		var data interface{}
		var err error
		var context string
		var size uint
		// var ll int

		start := time.Now()

		switch job := i.payload.(type) {
		case *HeadersJob:
			fmt.Printf("SYNC HEADERS (%s) (%d): %d, %d\n", p.id, i.id, job.block, job.count)

			// p.requestBandwidth(p.sched.getHeaderSize() * int(job.count))

			data, err = p.conn.RequestHeadersSync(ctx, job.block, job.count)
			fmt.Printf("DOWN HEADERS: (%s): %d\n", p.id, size)
			context = "headers"
		case *BodiesJob:
			fmt.Printf("SYNC BODIES (%s) (%d): %d\n", p.id, i.id, len(job.hashes))

			// p.requestBandwidth(p.sched.getHeaderSize() * len(job.hashes))

			data, err = p.conn.RequestBodiesSync(ctx, job.hash, job.hashes)
			fmt.Printf("DOWN BODIES: (%s): %d\n", p.id, size)
			context = "bodies"
		case *ReceiptsJob:
			fmt.Printf("SYNC RECEIPTS (%s) (%d): %d\n", p.id, i.id, len(job.hashes))

			// p.requestBandwidth(p.sched.getHeaderSize() * len(job.hashes))

			data, err = p.conn.RequestReceiptsSync(ctx, job.hash, job.hashes)
			fmt.Printf("DOWN RECEIPTS: (%s): %d\n", p.id, size)
			context = "receipts"
		}

		// Dont deliver any data
		if !p.running {
			return
		}

		end := time.Since(start)

		p.sched.Deliver("Y", context, i.id, data, err)
		metrics.SetGaugeWithLabels([]string{"minimal", "protocol", "ethereum63", "size"}, float32(size), []metrics.Label{{Name: "id", Value: p.id}})
		metrics.SetGaugeWithLabels([]string{"minimal", "protocol", "ethereum63", "time"}, float32(end.Nanoseconds()), []metrics.Label{{Name: "id", Value: p.id}})

		if err != nil {
			if strings.Contains(err.Error(), "session closed") {
				return
			}
			fmt.Printf("ERR: %v\n", err)
			time.Sleep(5 * time.Second)
		} else {
			// p.updateRate(int(size))
			// p.sched.updateApproxSize(context, int(size), ll)
		}

		time.Sleep(500 * time.Millisecond)
	}
}
