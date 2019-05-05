package ethereum

import (
	"container/heap"
	"context"
	"fmt"
	"math"
	"math/big"
	"net"

	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/minimal"
	"github.com/umbracle/minimal/sealer"

	"sync"
	"time"

	metrics "github.com/armon/go-metrics"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/protocol"
)

// Backend is the ethereum backend
type Backend struct {
	NetworkID  uint64
	minimal    *minimal.Minimal
	blockchain *blockchain.Blockchain
	queue      *queue

	counter int
	last    int

	peers     map[string]*Ethereum
	peersLock sync.Mutex
	waitCh    []chan struct{}

	deliverLock sync.Mutex
	watch       chan *NotifyMsg
	stopFn      context.CancelFunc

	// -- new fields
	heap   *workersHeap
	tasks  []*task
	wakeCh chan struct{}
	taskCh chan struct{}
}

func Factory(ctx context.Context, m interface{}, config map[string]interface{}) (protocol.Backend, error) {
	minimal := m.(*minimal.Minimal)
	return NewBackend(minimal, minimal.Blockchain)
}

// NewBackend creates a new ethereum backend
func NewBackend(minimal *minimal.Minimal, blockchain *blockchain.Blockchain) (*Backend, error) {
	b := &Backend{
		minimal:     minimal,
		peers:       map[string]*Ethereum{},
		peersLock:   sync.Mutex{},
		blockchain:  blockchain,
		queue:       newQueue(),
		counter:     0,
		last:        0,
		deliverLock: sync.Mutex{},
		waitCh:      make([]chan struct{}, 0),

		// -- new fields
		heap: &workersHeap{
			index: map[string]*worker{},
			heap:  workersHeapImpl{},
		},
		tasks:  []*task{},
		wakeCh: make(chan struct{}, 10),
		taskCh: make(chan struct{}, maxConcurrentTasks),
	}

	header, ok := blockchain.Header()
	if !ok {
		return nil, fmt.Errorf("header not found")
	}

	if minimal != nil {
		b.NetworkID = uint64(minimal.Chain().Params.ChainID)
	} else {
		b.NetworkID = 1
	}

	b.queue.front = b.queue.newItem(header.Number.Uint64() + 1)
	b.queue.head = header.Hash()

	fmt.Printf("Current header (%d): %s\n", header.Number.Uint64(), header.Hash().String())

	if minimal != nil {
		go b.WatchMinedBlocks(minimal.Sealer.SealedCh)
	}

	// start the process
	go b.runSync()

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

// -- setup --

func (b *Backend) addPeer(id string, proto *Ethereum) {
	fmt.Println("## ADD PEER ##")

	b.heap.Push(id, proto)
	b.notifyAvailablePeer()
}

func (b *Backend) peek() *worker {
	<-b.taskCh // acquire a task

PEEK:
	w := b.heap.Peek()
	if w != nil && w.outstanding < maxOutstandingRequests {
		return w
	}

	// Best peer has too many open requests. Wait until
	// there is some other to query

	select {
	case <-b.wakeCh:
		goto PEEK
	}
}

func (b *Backend) runTask(t *task) {
	// run logic
	failed := b.runTaskImpl(t)

	if !failed {
		outstanding, _ := b.heap.Update(t.peerID, -1)
		if outstanding == maxOutstandingRequests-1 {
			b.notifyAvailablePeer()
		}
	} else {
		// peer has failed, remove it from the list
		b.heap.Remove(t.peerID)
	}

	// release task
	b.taskCh <- struct{}{}
}

func (b *Backend) runTaskImpl(t *task) bool {
	id := t.peerID
	ctx := context.Background()

	var data interface{}
	var err error
	var context string

	conn, ok := b.peers[t.peerID]
	if !ok {
		panic("not found")
	}

	switch job := t.job.payload.(type) {
	case *HeadersJob:
		fmt.Printf("SYNC HEADERS (%s): %d %d\n", id, job.block, job.count)

		data, err = conn.RequestHeadersSync(ctx, job.block, job.count)
		context = "headers"
	case *BodiesJob:
		fmt.Printf("SYNC BODIES (%s): %d\n", id, len(job.hashes))

		data, err = conn.RequestBodiesSync(ctx, job.hash, job.hashes)
		context = "bodies"
	case *ReceiptsJob:
		fmt.Printf("SYNC RECEIPTS (%s): %d\n", id, len(job.hashes))

		data, err = conn.RequestReceiptsSync(ctx, job.hash, job.hashes)
		context = "receipts"
	}

	if err != nil {
		if err.Error() == "session closed" {
			// remove the peer from the list of peers if the session has been closed
			return true
		}
	}

	b.Deliver("Y", context, t.job.id, data, err)
	return false
}

func (b *Backend) notifyAvailablePeer() {
	select {
	case b.wakeCh <- struct{}{}:
	default:
	}
}

func (b *Backend) dequeueJob() *Job {
WAIT:
	job, err := b.queue.Dequeue()
	if err != nil {
		panic(err)
	}

	if job != nil {
		return job
	}

	// TODO, use a channel to nofify new jobs
	// That channel will be used by syncer and watcher
	time.Sleep(5 * time.Second)
	goto WAIT
}

func (b *Backend) runSync() {
	for i := 0; i < maxConcurrentTasks; i++ {
		b.taskCh <- struct{}{}
	}

	for {
		w := b.peek()

		fmt.Printf("Worker: %s\n", w.id)

		job := b.dequeueJob()

		t := &task{
			peerID: w.id,
			job:    job,
		}

		b.heap.Update(t.peerID, 1)
		b.tasks = append(b.tasks, t)

		go b.runTask(t)
	}
}

func (b *Backend) WatchMinedBlocks(watch chan *sealer.SealedNotify) {
	for {
		w := <-watch
		// TODO, change with a blockchain listener
		go b.broadcast(w.Block)
	}
}

func (b *Backend) broadcast(block *types.Block) {
	// total difficulty so far at the parent
	diff, ok := b.blockchain.GetTD(block.ParentHash())
	if !ok {
		// log error
		return
	}

	// total difficulty + the difficulty of the block
	blockDiff := big.NewInt(1).Add(diff, block.Difficulty())

	for _, i := range b.peers {
		if err := i.SendNewBlock(block, blockDiff); err != nil {
			fmt.Printf("Failed to send send block to peer %s: %v\n", i.peerID, err)
		}
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

/*
func (b *Backend) notifyNewData(w *NotifyMsg) {
	peerEth := b.peers[w.Peer.id].conn

	header, err := peerEth.fetchHeight(context.Background())
	if err != nil {
		fmt.Printf("ERR: fetch height failed: %v\n", err)
		return
	}

	ancestor, forkHeader, err := b.FindCommonAncestor(peerEth)
	if err != nil {
		panic(err)
	}

	ourHeader, _ := b.blockchain.Header()
	td, _ := b.blockchain.GetTD(ourHeader.Hash())

	// check that the difficulty is higher than ours
	if w.Diff.Cmp(td) < 0 {
		fmt.Printf("==> Difficulty %s is lower than ours %s, skip it\n", peerEth.HeaderDiff.String(), td.String())
	} else {
		// We discovered a new difficulty, start to query this one.

		if forkHeader == nil {
			// We are at the same block
			return
		}

		if ancestor.Hash().String() != forkHeader.ParentHash.String() {
			// ERROR.

			fmt.Printf("Fork: %s (%d)\n", forkHeader.Hash().String(), forkHeader.Number.Uint64())

			fmt.Println("-- fork parent --")
			fmt.Println(forkHeader.ParentHash.String())

			fmt.Println("-- ancestor (this should be fork parent) --")
			fmt.Println(ancestor.Hash().String())
			fmt.Println(ancestor.Number.Uint64())

			panic("error")
		}

		forkBlock := []*types.Block{
			types.NewBlockWithHeader(forkHeader),
		}
		if err := b.blockchain.WriteBlocks(forkBlock); err != nil {
			panic(err)
		}

		// Change the queue to start to download from the ancestor position
		b.queue = newQueue()
		b.queue.front = b.queue.newItem(ancestor.Number.Uint64() + 1)
		b.queue.head = ancestor.Hash()

		// Update the chain last header
		b.updateChain(header.Number.Uint64())

		// TODO: Reset peer connections to avoid receive bad values and start the whole process
		// Important to do it after the queue is defined
		for _, p := range b.peers {
			p.Reset()
		}
	}
}
*/

/*
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
*/

// Add is called when we connect to a new node
func (b *Backend) Add(conn net.Conn, peerID string) (protocol.Handler, error) {
	fmt.Println("----- ADD NODE -----")
	fmt.Println(peerID)

	// use handler to create the connection

	status, err := b.GetStatus()
	if err != nil {
		return nil, err
	}

	proto := NewEthereumProtocol(peerID, conn, b.blockchain)
	proto.backend = b

	b.peersLock.Lock()
	b.peers[peerID] = proto
	b.peersLock.Unlock()

	// Start the protocol handle
	if err := proto.Init(status); err != nil {
		return nil, err
	}

	// Only validate for the DAO Fork on Ethereum mainnet
	if b.NetworkID == 1 {
		if err := proto.ValidateDAOBlock(); err != nil {
			return nil, err
		}
	}

	header, err := proto.fetchHeight(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch height: %v", err)
	}

	b.updateChain(header.Number.Uint64())
	b.addPeer(peerID, proto)

	return proto, nil

	/*
		peerConn := &PeerConnection{
			conn:    proto,
			sched:   b,
			id:      peerID,
			enabled: true,
		}

		b.peers[peerID] = peerConn
		proto.peer = peerConn

		// notifiy this node data
		b.notifyNewData(&NotifyMsg{
			Peer: peerConn,
			Diff: proto.HeaderDiff,
		})

		return proto, nil
	*/
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
		// TODO, we need to set here the thing that was not deliver as waiting to be dequeued again
		fmt.Printf("=> Failed to deliver (%d): %v\n", id, err)
		if err := b.queue.updateFailedElem(peer, id, context); err != nil {
			fmt.Printf("Could not be updated: %v\n", err)
		}
		return
	}

	switch obj := data.(type) {
	case []*types.Header:
		fmt.Printf("deliver headers %d: %d\n", id, len(obj))
		if err := b.queue.deliverHeaders(id, obj); err != nil {
			panic(fmt.Errorf("Failed to deliver headers (%d): %v", id, err))
		}

	case []*types.Body:
		fmt.Printf("deliver bodies %d: %d\n", id, len(obj))
		if err := b.queue.deliverBodies(id, obj); err != nil {
			panic(fmt.Errorf("Failed to deliver bodies (%d): %v", id, err))
		}

	case [][]*types.Receipt:
		fmt.Printf("deliver receipts %d: %d\n", id, len(obj))
		if err := b.queue.deliverReceipts(id, obj); err != nil {
			panic(fmt.Errorf("Failed to deliver receipts (%d): %v", id, err))
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

	fmt.Println("-- batches --")
	fmt.Println(b.queue.NumOfCompletedBatches())

	if b.queue.NumOfCompletedBatches() >= 5 { // force to commit data every time
		data := b.queue.FetchCompletedData()

		fmt.Printf("Commit data: %d\n", len(data)*maxElements)
		fmt.Printf("New Head: %s\n", b.queue.head.String())

		// write the headers
		for indx, elem := range data {
			metrics.SetGauge([]string{"minimal", "protocol", "ethereum63", "block"}, float32(elem.headers[0].Number.Uint64()))

			// we have to use writeblcoks because that one changes the state
			blocks := []*types.Block{}
			for _, i := range elem.headers {
				blocks = append(blocks, types.NewBlockWithHeader(i))
			}

			// Add transactions and uncles
			for indx, i := range elem.bodiesHeaders {
				body := elem.bodies[indx]
				blocks[i] = blocks[i].WithBody(body.Transactions, body.Uncles)
			}

			// Write blocks and commit new data
			if err := b.blockchain.WriteBlocks(blocks); err != nil {
				fmt.Printf("Failed to write headers batch: %v", err)

				first, last := elem.headers[0], elem.headers[len(elem.headers)-1]

				fmt.Printf("Error at step: %d\n", indx)
				fmt.Printf("First block we have is %s (%s) %s (%s)\n", first.Hash().String(), first.Number.String(), last.Hash().String(), last.Number.String())

				panic("")
				return
			}

			h, _ := b.blockchain.Header()
			fmt.Printf("New header number: %d\n", h.Number.Uint64())
		}
	}
}

// GetStatus returns the current ethereum status
func (b *Backend) GetStatus() (*Status, error) {
	header, ok := b.blockchain.Header()
	if !ok {
		return nil, fmt.Errorf("header not found")
	}

	// We transmit the total difficulty of our current chain
	td, ok := b.blockchain.GetTD(header.Hash())
	if !ok {
		return nil, fmt.Errorf("header difficulty not found")
	}

	status := &Status{
		ProtocolVersion: 63,
		NetworkID:       b.NetworkID,
		TD:              td,
		CurrentBlock:    header.Hash(),
		GenesisBlock:    b.blockchain.Genesis().Hash(),
	}
	return status, nil
}

// FindCommonAncestor finds the common ancestor with the peer and the syncer connection
func (b *Backend) FindCommonAncestor(peer *Ethereum) (*types.Header, *types.Header, error) {
	// Binary search, TODO, works but it may take a lot of time

	h, _ := b.blockchain.Header()

	min := 0 // genesis
	max := int(h.Number.Uint64())

	height, err := peer.fetchHeight(context.Background())
	if err != nil {
		return nil, nil, err
	}
	if heightNumber := int(height.Number.Uint64()); max > heightNumber {
		max = heightNumber
	}

	var header *types.Header
	var ok bool

	ctx := context.Background()
	for min <= max {
		m := uint64(math.Floor(float64(min+max) / 2))

		found, ok, err := peer.RequestHeaderSync(ctx, m)
		if err != nil {
			return nil, nil, err
		}

		if !ok {
			// peer does not have the m peer, search in lower bounds
			max = int(m - 1)
		} else {
			if found.Number.Uint64() != m {
				return nil, nil, fmt.Errorf("header response number not correct, asked %d but retrieved %d", m, header.Number.Uint64())
			}

			expectedHeader, ok := b.blockchain.GetHeaderByNumber(big.NewInt(int64(m)))
			if !ok {
				return nil, nil, fmt.Errorf("cannot find the header in local chain")
			}

			if expectedHeader.Hash() == found.Hash() {
				header = found
				min = int(m + 1)
			} else {
				max = int(m - 1)
			}
		}
	}

	if min == 0 {
		return nil, nil, nil
	}

	// Get next element, that would be the index of the fork
	if height.Number.Uint64() == header.Number.Uint64() {
		return header, nil, nil
	}

	fork, ok, err := peer.RequestHeaderSync(ctx, header.Number.Uint64()+1)
	if err != nil {
		return nil, nil, err
	}
	if !ok { // Can this happen if we check the height point?
		return nil, nil, fmt.Errorf("fork point not found")
	}

	return header, fork, nil
}

const (
	maxOutstandingRequests = 4
	maxConcurrentTasks     = 10
)

// worker represents a queryable peer
type worker struct {
	id          string
	index       int
	outstanding int
	proto       *Ethereum
}

type workersHeap struct {
	index map[string]*worker
	heap  workersHeapImpl
}

func (w *workersHeap) Remove(id string) {
	wrk, ok := w.index[id]
	if !ok {
		return
	}
	heap.Remove(&w.heap, wrk.index)
	return
}

func (w *workersHeap) Push(id string, proto *Ethereum) error {
	if _, ok := w.index[id]; ok {
		return fmt.Errorf("peer '%s' already exists", id)
	}

	wrk := &worker{
		id:          id,
		index:       0,
		proto:       proto,
		outstanding: 0, // start with zero outstanding requests
	}
	w.index[id] = wrk
	heap.Push(&w.heap, wrk)
	return nil
}

func (w *workersHeap) Update(id string, outstanding int) (int, error) {
	wrk, ok := w.index[id]
	if !ok {
		return 0, fmt.Errorf("peer '%s' not found", id)
	}

	wrk.outstanding += outstanding
	heap.Fix(&w.heap, wrk.index)
	return wrk.outstanding, nil
}

func (w *workersHeap) Peek() *worker {
	if len(w.heap) == 0 {
		return nil
	}
	return w.heap[0]
}

type workersHeapImpl []*worker

func (w workersHeapImpl) Len() int {
	return len(w)
}

func (w workersHeapImpl) Less(i, j int) bool {
	return w[i].outstanding < w[j].outstanding
}

func (w workersHeapImpl) Swap(i, j int) {
	w[i], w[j] = w[j], w[i]
	w[i].index = i
	w[j].index = j
}

func (w *workersHeapImpl) Push(x interface{}) {
	n := len(*w)
	pJob := x.(*worker)
	pJob.index = n
	*w = append(*w, pJob)
}

func (w *workersHeapImpl) Pop() interface{} {
	old := *w
	n := len(old)
	pJob := old[n-1]
	pJob.index = -1
	*w = old[0 : n-1]
	return pJob
}

type task struct {
	job    *Job
	peerID string
}
