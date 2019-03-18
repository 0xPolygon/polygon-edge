package syncer

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/armon/go-metrics"

	"github.com/umbracle/minimal/network"
	"github.com/umbracle/minimal/protocol/ethereum"
)

type PeerConnection struct {
	quota  int
	sched  *Syncer
	conn   *ethereum.Ethereum
	peerID string
	peer   *network.Peer

	rateLock sync.Mutex
	rate     int
	counter  int
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
		metrics.SetGaugeWithLabels([]string{"syncer", "rate_2"}, float32(p.rate), []metrics.Label{{Name: "id", Value: p.peerID}})
		metrics.SetGaugeWithLabels([]string{"syncer", "rate"}, float32(sample), []metrics.Label{{Name: "id", Value: p.peerID}})
		// bytes/second

		fmt.Printf("==> Rate (%s): %d\n", p.peerID, p.rate)
		p.counter = 0
		p.rateLock.Unlock()
	}
}

func (p *PeerConnection) updateRate(bytes int) {
	fmt.Printf("Update rate (%s): %d\n", p.peerID, bytes)

	p.rateLock.Lock()
	defer p.rateLock.Unlock()

	p.counter += bytes
}

func (p *PeerConnection) Run() {

	go p.run()

	/*
		BACK:
			fmt.Println("trying to fetch")
			header, err := p.sched.FetchHeight(p.conn)
			if err != nil {
				time.Sleep(5 * time.Second)
				goto BACK
			}

			metrics.SetGaugeWithLabels([]string{"syncer", "header"}, float32(header.Number.Uint64()), []metrics.Label{{Name: "id", Value: p.peerID}})
	*/

	go p.action()
	// go p.action()

	// go p.action()
}

func (p *PeerConnection) action() {
	fmt.Println("############## PEER CONNECTION HAS STARTED #################")

	for {
		i := p.sched.Dequeue()

		var data interface{}
		var err error
		var context string
		var size uint
		// var ll int

		start := time.Now()

		switch job := i.payload.(type) {
		case *HeadersJob:
			fmt.Printf("SYNC HEADERS (%s) (%d): %d, %d\n", p.peerID, i.id, job.block, job.count)

			// p.requestBandwidth(p.sched.getHeaderSize() * int(job.count))

			data, err = p.conn.RequestHeadersSync(job.block, job.count)
			fmt.Printf("DOWN HEADERS: (%s): %d\n", p.peerID, size)
			context = "headers"
		case *BodiesJob:
			fmt.Printf("SYNC BODIES (%s) (%d): %d\n", p.peerID, i.id, len(job.hashes))

			// p.requestBandwidth(p.sched.getHeaderSize() * len(job.hashes))

			data, err = p.conn.RequestBodiesSync(job.hash, job.hashes)
			fmt.Printf("DOWN BODIES: (%s): %d\n", p.peerID, size)
			context = "bodies"
		case *ReceiptsJob:
			fmt.Printf("SYNC RECEIPTS (%s) (%d): %d\n", p.peerID, i.id, len(job.hashes))

			// p.requestBandwidth(p.sched.getHeaderSize() * len(job.hashes))

			data, err = p.conn.RequestReceiptsSync(job.hash, job.hashes)
			fmt.Printf("DOWN RECEIPTS: (%s): %d\n", p.peerID, size)
			context = "receipts"
		}

		end := time.Since(start)

		p.sched.Deliver("Y", context, i.id, data, err)
		metrics.SetGaugeWithLabels([]string{"syncer", "size"}, float32(size), []metrics.Label{{Name: "id", Value: p.peerID}})
		metrics.SetGaugeWithLabels([]string{"syncer", "time"}, float32(end.Nanoseconds()), []metrics.Label{{Name: "id", Value: p.peerID}})

		if err != nil {
			if strings.Contains(err.Error(), "session closed") {

				/*
					fmt.Println("---- DOPE ----")
					connectedAt := p.peer.ConnectedAt()

					fmt.Println(p.peerID)
					fmt.Println(connectedAt)
					fmt.Println(time.Since(connectedAt))
					fmt.Println(p.peer.Info.Name)

					// panic("X")
				*/

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
