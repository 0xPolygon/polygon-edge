package ethereum

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"net"
	"sort"

	"github.com/umbracle/minimal/minimal"

	// "strconv"
	"sync"
	"time"

	metrics "github.com/armon/go-metrics"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/protocol"
	"github.com/umbracle/minimal/sealer"
)

type Config struct {
	MaxRequests int
	NumWorkers  int
}

func DefaultConfig() *Config {
	c := &Config{
		MaxRequests: 5,
		NumWorkers:  2,
	}
	return c
}

// Blockchain is the reference the syncer needs to connect to the blockchain
type Blockchain interface {
	Header() *types.Header
	Genesis() *types.Header
	WriteHeaders(headers []*types.Header) error
	CommitBodies(headers []common.Hash, bodies []*types.Body) error
	CommitReceipts(headers []common.Hash, receipts []types.Receipts) error
	GetHeaderByHash(hash common.Hash) *types.Header
	GetHeaderByNumber(n *big.Int) *types.Header
	GetReceiptsByHash(hash common.Hash) types.Receipts
	GetBodyByHash(hash common.Hash) *types.Body
}

type Peer struct {
	pretty  string
	id      string
	conn    *Ethereum
	active  bool
	failed  int
	pending int
}

// TODO, test the syncer without creating server connections.
// TODO, Is it possible to have differents versions of the same protocol
// running at the same time?

func newPeer(id string, conn *Ethereum) *Peer {
	return &Peer{
		id:     id,
		active: true,
		pretty: id,
		conn:   conn,
	}
}

// Backend is the ethereum backend
type Backend struct {
	NetworkID uint64
	config    *Config

	blockchain Blockchain
	queue      *queue

	counter int
	last    int

	peers     map[string]*Peer
	peersLock sync.Mutex
	waitCh    []chan struct{}

	deliverLock sync.Mutex

	/*
		// bandwidth
		bandwidthLock sync.Mutex
		bandwidth     int
	*/

	watch chan *NotifyMsg

	/*
		// sizes
		sizesLock     sync.Mutex
		headerSize    int
		headerCount   int
		bodySize      int
		bodyCount     int
		receiptSize   int
		receiptsCount int
	*/
}

func Factory(ctx context.Context, m interface{}) (*Backend, error) {
	minimal := m.(*minimal.Minimal)
	return NewBackend(1, minimal.Blockchain, DefaultConfig())
}

// NewBackend creates a new ethereum backend
func NewBackend(networkID uint64, blockchain Blockchain, config *Config) (*Backend, error) {
	b := &Backend{
		config:      config,
		NetworkID:   networkID,
		peers:       map[string]*Peer{},
		peersLock:   sync.Mutex{},
		blockchain:  blockchain,
		queue:       newQueue(),
		counter:     0,
		last:        0,
		deliverLock: sync.Mutex{},
		waitCh:      make([]chan struct{}, 0),
		watch:       make(chan *NotifyMsg, 100),
		/*
			sizesLock:     sync.Mutex{},
			headerSize:    300,
			headerCount:   1,
			bodySize:      300,
			bodyCount:     1,
			receiptSize:   300,
			receiptsCount: 1,
			bandwidthLock: sync.Mutex{},
		*/
	}

	header := blockchain.Header()

	fmt.Println(header)

	b.queue.front = b.queue.newItem(header.Number.Uint64() + 1)
	b.queue.head = header.Hash()

	// Maybe start s.back as s.front and calls to dequeue would block

	fmt.Printf("Current header (%d): %s\n", header.Number.Uint64(), header.Hash().String())

	// go s.refreshBandwidth()
	go b.watchBlockLoop()

	return b, nil
}

// ETH63 is the Fast synchronization protocol
var ETH63 = protocol.Protocol{
	Name:    "eth",
	Version: 63,
	Length:  17,
}

func (b *Backend) Protocol() protocol.Protocol {
	return ETH63
}

func (b *Backend) watchMinedBlocks(watch chan sealer.SealedNotify) {
	go func() {
		for {
			w := <-watch

			fmt.Println("-- watch --")
			fmt.Println(w)

			// Notify that a new block has been sealed
			// the blockchain has been update the most likely
			// What if we dont get the notification from the sealer but
			// from the blockchain? Every time there is an update
			// so that we dont have communication between the sealer
			// and the syncer and just with the blockchain. The same way that
			// sealer talks with the blockchain to know about updates!

			// Not sure if it has to recompute some of the data.
			// This is only called when the sealer is activated so it does not need
			// to be in 'syncer' mode but in a more limited 'watch' mode.

			// For now just broadcast all the nodes that arrve here.
		}
	}()
}

func (b *Backend) broadcast(block *types.Block) {
	for _, i := range b.peers {
		i.conn.SendNewBlock(block, big.NewInt(1))
	}
}

func (b *Backend) updateChain(block uint64) {
	// updates the back object
	if b.queue.back == nil {
		b.queue.addBack(block)
	} else {
		if block > b.queue.back.block {
			b.queue.addBack(block)
		}
	}
}

func (b *Backend) watchBlockLoop() {
	for {
		w := <-b.watch
		// there is an update from one of the peers
		// NOTE: do some operations to check the healthiness of the update
		// For example, check if we receive too many updates from same peer.

		b.notifyNewData(w)
	}
}

// TODO, sendBlocks for the notification does not send the full difficulty

func (b *Backend) notifyNewData(w *NotifyMsg) {
	fmt.Println(w)

	peerEth := b.peers[w.Peer.ID].conn

	// check if difficulty is higher than ours
	// it its higher do the check about the header and ancestor

	// find data about the peer
	header, err := b.fetchHeight(peerEth)
	if err != nil {
		fmt.Printf("ERR: fetch height failed: %v\n", err)
		return
	}

	fmt.Printf("Heigth: %d\n", header.Number.Uint64())

	ancestor, err := b.FindCommonAncestor(peerEth)
	if err != nil {
		return
	}

	fmt.Printf("Ancestor: %d\n", ancestor.Number.Uint64())

	ourHeader := b.blockchain.Header() // keep this header locally? maybe return the last header from blockchain

	// check that the difficulty is higher than ours
	if peerEth.HeaderDiff.Cmp(ourHeader.Difficulty) < 0 {
		fmt.Printf("Difficulty %s is lower than ours %s, skip it\n", peerEth.HeaderDiff.String(), b.blockchain.Header().Difficulty.String())
	} else {
		fmt.Println("Difficulty higher than ours")
		b.updateChain(header.Number.Uint64())
	}
}

func (b *Backend) watchBlock(watch chan *NotifyMsg) {
	go func() {
		for {
			w := <-watch
			select {
			case b.watch <- w:
			default:
			}
		}
	}()
}

// Add is called when we connect to a new node
func (b *Backend) Add(conn net.Conn, peerID string) error {
	fmt.Println("----- ADD NODE -----")

	/*
		if err := s.checkDAOHardFork(peer.GetProtocol("eth", 63).(*Ethereum)); err != nil {
			fmt.Println("Failed to check the DAO block")
			return
		}
		fmt.Println("DAO Fork completed")
	*/

	// use handler to create the connection

	proto := NewEthereumProtocol(conn, b.GetStatus, b.blockchain)

	// Start the protocol handle
	if err := proto.Init(); err != nil {
		return err
	}

	p := newPeer(peerID, proto)
	b.peers[peerID] = p

	// wake up some task
	// s.wakeUp()

	// b.watchBlock(peerEth.Watch())

	peerConn := PeerConnection{
		conn:   proto,
		sched:  b,
		peerID: peerID,
	}
	peerConn.Run()

	/*
		// notifiy this node data
		b.notifyNewData(&NotifyMsg{
			Peer: peer,
			Diff: peerEth.HeaderDiff,
		})
	*/

	// go s.runPeer(p)
	return nil
}

func (b *Backend) workerTask(id string) {
	fmt.Printf("Worker task starts: %s\n", id)

	for {
		peer := b.dequeuePeer()

		i := b.Dequeue()

		var data interface{}
		var err error
		var context string

		switch job := i.payload.(type) {
		case *HeadersJob:
			fmt.Printf("SYNC HEADERS (%s) (%d) (%s): %d, %d\n", id, i.id, peer.pretty, job.block, job.count)

			data, err = peer.conn.RequestHeadersSync(job.block, job.count)
			context = "headers"
		case *BodiesJob:
			fmt.Printf("SYNC BODIES (%s) (%d) (%s): %d\n", id, i.id, peer.pretty, len(job.hashes))

			data, err = peer.conn.RequestBodiesSync(job.hash, job.hashes)
			context = "bodies"
		case *ReceiptsJob:
			fmt.Printf("SYNC RECEIPTS (%s) (%d) (%s): %d\n", id, i.id, peer.pretty, len(job.hashes))

			data, err = peer.conn.RequestReceiptsSync(job.hash, job.hashes)
			context = "receipts"
		}

		// ack the job and enqueue again for more work
		b.ack(peer, err != nil)

		b.Deliver(peer.pretty, context, i.id, data, err)

		fmt.Printf("Completed: %d\n", b.queue.NumOfCompletedBatches())
	}
}

// Run is the main entry point
func (b *Backend) Run() {
	/*
		for i := 0; i < s.config.NumWorkers; i++ {
			go s.workerTask(strconv.Itoa(i))
		}
	*/
}

type peers []*Peer

func (p peers) Len() int {
	return len(p)
}

func (p peers) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func (p peers) Less(i, j int) bool {
	if p[i].failed < p[j].failed {
		return true
	}
	return p[i].pending < p[j].pending
}

func (b *Backend) dequeuePeer() *Peer {
SELECT:
	//fmt.Println("-- getting peers --")
	b.peersLock.Lock()
	pp := peers{}
	for _, i := range b.peers {
		//fmt.Printf("Peer: %s %v %d %v\n", i.pretty, i.active, i.pending, i.pending <= s.config.MaxRequests-1)
		if i.active && i.pending <= b.config.MaxRequests-1 {
			pp = append(pp, i)
		}
	}
	sort.Sort(pp)

	//fmt.Println("-- pending --")
	//fmt.Println(pp)

	if len(pp) > 0 {
		candidate := pp[0]
		candidate.pending++

		b.peersLock.Unlock()
		return candidate
	}

	b.peersLock.Unlock()

	fmt.Println("- nothign found go to sleep -")

	// wait and sleep
	wait := make(chan struct{})
	b.waitCh = append(b.waitCh, wait)

	for {
		<-wait
		fmt.Println("- awake now -")
		goto SELECT
	}
}

func (b *Backend) ack(peer *Peer, failed bool) {
	b.peersLock.Lock()

	// acknoledge a task has finished
	peer.pending--

	if failed {
		peer.failed++
	}
	if peer.failed == 10 {
		peer.active = false
		b.retryAfter(peer, 5*time.Second)
	}
	b.peersLock.Unlock()

	if peer.active {
		for i := peer.pending; i < b.config.MaxRequests; i++ {
			b.wakeUp()
		}
	}
}

func (b *Backend) wakeUp() {
	//fmt.Println("- wake up -")
	//fmt.Println(len(s.waitCh))

	// wake up a task if necessary
	if len(b.waitCh) > 0 {
		var wake chan struct{}
		wake, b.waitCh = b.waitCh[0], b.waitCh[1:]

		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

func (b *Backend) Dequeue() *Job {
	job, err := b.queue.Dequeue()
	if err != nil {
		fmt.Printf("Failed to dequeue: %v\n", err)
	}
	return job
}

func (b *Backend) Deliver(peer string, context string, id uint32, data interface{}, err error) {
	b.deliverLock.Lock()
	defer b.deliverLock.Unlock()

	if err != nil {
		// log
		// TODO, we need to set here the thing that was not deliver as waiting to be dequeued again
		fmt.Printf("==================================> Failed to deliver (%d): %v\n", id, err)
		if err := b.queue.updateFailedElem(peer, id, context); err != nil {
			fmt.Printf("Could not be updated: %v\n", err)
		}
		return
	}

	switch obj := data.(type) {
	case []*types.Header:
		fmt.Printf("deliver headers %d: %d\n", id, len(obj))
		if err := b.queue.deliverHeaders(id, obj); err != nil {
			fmt.Printf("Failed to deliver headers (%d): %v\n", id, err)

			panic("")
		}

	case []*types.Body:
		fmt.Printf("deliver bodies %d: %d\n", id, len(obj))
		if err := b.queue.deliverBodies(id, obj); err != nil {
			fmt.Printf("Failed to deliver bodies (%d): %v\n", id, err)

			panic("")
		}

	case [][]*types.Receipt:
		fmt.Printf("deliver receipts %d: %d\n", id, len(obj))
		if err := b.queue.deliverReceipts(id, obj); err != nil {
			fmt.Printf("Failed to deliver receipts (%d): %v\n", id, err)

			panic("")
		}

	default:
		panic(data)
	}

	fmt.Println(b.queue.NumOfCompletedBatches())
	if n := b.queue.NumOfCompletedBatches(); n == b.last {
		b.counter++
	} else {
		b.last = n
		b.counter = 0
	}

	if b.counter == 500 {
		b.queue.printQueue()
		panic("")
	}

	if b.queue.NumOfCompletedBatches() > 2 {
		data := b.queue.FetchCompletedData()

		fmt.Printf("Commit data: %d\n", len(data)*maxElements)
		fmt.Printf("New Head: %s\n", b.queue.head.String())

		// write the headers
		for indx, elem := range data {
			metrics.SetGauge([]string{"syncer", "block"}, float32(elem.headers[0].Number.Uint64()))

			if err := b.blockchain.WriteHeaders(elem.headers); err != nil {
				fmt.Printf("Failed to write headers batch: %v", err)

				first, last := elem.headers[0], elem.headers[len(elem.headers)-1]

				fmt.Printf("Error at step: %d\n", indx)
				fmt.Printf("First block we have is %s (%s) %s (%s)\n", first.Hash().String(), first.Number.String(), last.Hash().String(), last.Number.String())

				panic("")
				return
			}
			if err := b.blockchain.CommitBodies(elem.GetBodiesHashes(), elem.bodies); err != nil {
				fmt.Printf("Failed to write bodies: %v", err)
				panic("")
			}
			if err := b.blockchain.CommitReceipts(elem.GetReceiptsHashes(), elem.receipts); err != nil {
				fmt.Printf("Failed to write receipts: %v", err)
				panic("")
			}
		}
	}
}

// GetStatus returns the current ethereum status
func (b *Backend) GetStatus() (*Status, error) {
	header := b.blockchain.Header()

	status := &Status{
		ProtocolVersion: 63,
		NetworkID:       b.NetworkID,
		TD:              header.Difficulty,
		CurrentBlock:    header.Hash(),
		// Hardcoded for now
		GenesisBlock: common.HexToHash("0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3"),
	}
	return status, nil
}

var (
	daoBlock            = uint64(1920000)
	daoChallengeTimeout = 5 * time.Second
)

func (b *Backend) checkDAOHardFork(eth *Ethereum) error {
	return nil // hack

	if b.NetworkID == 1 {
		/*
			ack := make(chan rlpx.AckMessage, 1)
			eth.Conn().SetHandler(BlockHeadersMsg, ack, daoChallengeTimeout)

			// check the DAO block
			if err := eth.RequestHeadersByNumber(daoBlock, 1, 0, false); err != nil {
				return err
			}

			resp := <-ack
			if resp.Complete {
				var headers []*types.Header
				if err := rlp.Decode(resp.Payload, &headers); err != nil {
					return err
				}

				// TODO. check that daoblock is correct
				fmt.Println(headers)

			} else {
				return fmt.Errorf("timeout")
			}
		*/
	}

	return nil
}

func (b *Backend) retryAfter(p *Peer, d time.Duration) {
	time.AfterFunc(d, func() {
		b.peersLock.Lock()
		p.active = true
		b.wakeUp()
		b.peersLock.Unlock()
	})
}

// FindCommonAncestor finds the common ancestor with the peer and the syncer connection
func (b *Backend) FindCommonAncestor(peer *Ethereum) (*types.Header, error) {
	// Binary search, TODO, works but it may take a lot of time

	min := 0 // genesis
	max := int(b.blockchain.Header().Number.Uint64())

	height, err := b.fetchHeight(peer)
	if err != nil {
		return nil, err
	}
	if heightNumber := int(height.Number.Uint64()); max > heightNumber {
		max = heightNumber
	}

	var header *types.Header

	for min <= max {
		m := uint64(math.Floor(float64(min+max) / 2))

		headers, err := peer.RequestHeadersSync(m, 1)
		if err != nil {
			return nil, err
		}

		l := len(headers)
		if l == 0 {
			// peer does not have the m peer, search in lower bounds
			max = int(m - 1)
		} else if l == 1 {
			header = headers[0]
			if header.Number.Uint64() != m {
				return nil, fmt.Errorf("header response number not correct, asked %d but retrieved %d", m, header.Number.Uint64())
			}

			expectedHeader := b.blockchain.GetHeaderByNumber(big.NewInt(int64(m)))
			if expectedHeader == nil {
				return nil, fmt.Errorf("cannot find the header in local chain")
			}

			if expectedHeader.Hash() == header.Hash() {
				min = int(m + 1)
			} else {
				max = int(m - 1)
			}
		} else {
			return nil, fmt.Errorf("expected either 1 or 0 headers")
		}
	}

	if min == 0 {
		return nil, nil
	}
	return header, nil
}

func (b *Backend) FetchHeight(peer *Ethereum) (*types.Header, error) {
	return b.fetchHeight(peer)
}

// fetchHeight returns the header of the head hash of the peer
func (b *Backend) fetchHeight(peer *Ethereum) (*types.Header, error) {
	head := peer.Header()

	fmt.Println("-- head --")
	fmt.Println(head)

	header, err := peer.RequestHeaderByHashSync(head)
	if err != nil {
		return nil, err
	}
	if header == nil {
		return nil, fmt.Errorf("header not found")
	}
	if header.Hash() != head {
		return nil, fmt.Errorf("returned hash is not the correct one")
	}
	return header, nil
}

/*
// TODO, measure this every n times (i.e. epoch)
func (b *Backend) updateApproxSize(t string, size int, n int) {
	// fmt.Printf("T %s, S %d, L %d, Total %d\n", t, size, n, size/n)

	ss := size / n

	switch t {
	case "headers":
		s.headerSize = s.headerSize + (ss-s.headerSize)/s.headerCount
		s.headerCount++
		// fmt.Printf("Header size: %d\n", s.headerSize)
	case "bodies":
		s.bodySize = s.bodySize + (ss-s.bodySize)/s.bodyCount
		s.bodyCount++
		// fmt.Printf("Body size: %d\n", s.bodySize)
	case "receipts":
		s.receiptSize = s.receiptSize + (ss-s.receiptSize)/s.receiptsCount
		s.receiptsCount++
		// fmt.Printf("Receipts size: %d\n", s.receiptSize)
	}
}

func (b *Backend) requestBandwidth(bytes int) (int, int) {
	s.bandwidthLock.Lock()
	defer s.bandwidthLock.Unlock()

	if bytes > s.bandwidth {
		return 0, s.bandwidth
	}
	s.bandwidth -= bytes
	return bytes, s.bandwidth
}

func (b *Backend) getHeaderSize() int {
	s.sizesLock.Lock()
	defer s.sizesLock.Unlock()
	return s.headerSize
}

func (b *Backend) getBodiesSize() int {
	s.sizesLock.Lock()
	defer s.sizesLock.Unlock()
	return s.bodySize
}

func (b *Backend) getReceiptsSize() int {
	s.sizesLock.Lock()
	defer s.sizesLock.Unlock()
	return s.receiptSize
}

func (b *Backend) refreshBandwidth() {
	for {
		s.bandwidthLock.Lock()

		s.bandwidth += 256000
		if s.bandwidth > 256000 {
			s.bandwidth = 256000
		}

		s.bandwidthLock.Unlock()
		time.Sleep(1 * time.Second)
	}
}
*/
