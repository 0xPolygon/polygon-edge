package ethereum

import (
	"container/heap"
	"context"
	"fmt"
	"math"
	"math/big"
	"net"
	"sync/atomic"

	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/minimal"
	"github.com/umbracle/minimal/sealer"

	"sync"
	"time"

	protoCommon "github.com/umbracle/minimal/network/common"

	metrics "github.com/armon/go-metrics"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/protocol"
)

// Backend is the ethereum backend
type Backend struct {
	NetworkID  uint64
	minimal    *minimal.Minimal
	blockchain *blockchain.Blockchain
	queue      *queue

	logger hclog.Logger

	seq     uint64
	counter int
	last    int

	peers     map[string]*Ethereum
	peersLock sync.Mutex
	waitCh    []chan struct{}

	deliverLock sync.Mutex
	watch       chan *NotifyMsg
	stopFn      context.CancelFunc

	// -- new fields
	heap *workersHeap

	tasks     map[uint64]*task
	tasksLock sync.Mutex

	wakeCh   chan struct{}
	taskCh   chan struct{}
	commitCh chan []*element
}

func Factory(ctx context.Context, logger hclog.Logger, m interface{}, config map[string]interface{}) (protocol.Backend, error) {
	minimal := m.(*minimal.Minimal)
	return NewBackend(minimal, logger, minimal.Blockchain)
}

// NewBackend creates a new ethereum backend
func NewBackend(minimal *minimal.Minimal, logger hclog.Logger, blockchain *blockchain.Blockchain) (*Backend, error) {
	b := &Backend{
		logger:      logger,
		minimal:     minimal,
		peers:       map[string]*Ethereum{},
		peersLock:   sync.Mutex{},
		blockchain:  blockchain,
		queue:       newQueue(),
		counter:     0,
		last:        0,
		seq:         0,
		deliverLock: sync.Mutex{},
		waitCh:      make([]chan struct{}, 0),

		// -- new fields
		heap:      newWorkersHeap(),
		tasks:     map[uint64]*task{},
		tasksLock: sync.Mutex{},
		wakeCh:    make(chan struct{}, 10),
		taskCh:    make(chan struct{}, maxConcurrentTasks),
		commitCh:  make(chan []*element, 10),
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

	logger.Info("Header", "num", header.Number.Uint64(), "hash", header.Hash().String())

	if minimal != nil {
		go b.WatchMinedBlocks(minimal.Sealer.SealedCh)
	}

	// Create the task slots
	for i := 0; i < maxConcurrentTasks; i++ {
		b.taskCh <- struct{}{}
	}

	return b, nil
}

// ETH63 is the Fast synchronization protocol
var ETH63 = protoCommon.ProtocolSpec{
	Name:    "eth",
	Version: 63,
	Length:  17,
}

// Protocols implements the protocol interface
func (b *Backend) Protocols() []*protoCommon.Protocol {
	return []*protoCommon.Protocol{
		&protoCommon.Protocol{
			Spec:      ETH63,
			HandlerFn: b.Add,
		},
	}
}

// Run implements the protocol interface
func (b *Backend) Run() {
	go b.runSync()
	go b.commitData()
}

// -- setup --

func (b *Backend) addPeer(id string, proto *Ethereum) {
	b.heap.Push(id, proto)
	b.notifyAvailablePeer()
}

func (b *Backend) peek() *worker {
	<-b.taskCh // acquire a task

PEEK:
	w := b.heap.Peek()
	if w != nil && w.outstanding < maxOutstandingRequests {
		// Increse the number of outstanding requests
		b.heap.Update(w.id, 1)
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
	failed := t.Run()

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
	b.cleanupTask(t.seq)
}

func (b *Backend) cleanupTask(seq uint64) {
	b.tasksLock.Lock()
	delete(b.tasks, seq)
	b.tasksLock.Unlock()

	// release the task
	b.taskCh <- struct{}{}
}

func (b *Backend) cleanupAll() {
	b.tasksLock.Lock()
	defer b.tasksLock.Unlock()

	for _, t := range b.tasks {
		t.Close()

		// release the task
		b.taskCh <- struct{}{}
	}
	b.tasks = make(map[uint64]*task)
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
	for {
		w := b.peek()

		job := b.dequeueJob()

		seq := b.getSeq()

		t := &task{
			seq:     seq,
			peerID:  w.id,
			job:     job,
			backend: b,
			proto:   w.proto,
		}

		b.tasksLock.Lock()
		b.tasks[seq] = t
		b.tasksLock.Unlock()

		go b.runTask(t)
	}
}

func (b *Backend) WatchMinedBlocks(watch chan *sealer.SealedNotify) {
	for {
		w := <-watch
		// TODO, change with a blockchain listener
		go b.broadcastBlockMsg(w.Block)
	}
}

func (b *Backend) broadcastHashMsg(hash common.Hash, number uint64) {
	// TODO, should the access to b.peers be locked?

	for _, i := range b.peers {
		if err := i.SendNewHash(hash, number); err != nil {
			fmt.Printf("failed to propagate hash and number: %v\n", err)
		}
	}
}

func (b *Backend) broadcastBlockMsg(block *types.Block) {
	// total difficulty so far at the parent
	diff, ok := b.blockchain.GetTD(block.ParentHash())
	if !ok {
		// log error
		return
	}

	// total difficulty + the difficulty of the block
	blockDiff := big.NewInt(1).Add(diff, block.Difficulty())

	// TODO, broadcast to only a subset of the peers

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

// Add is called when we connect to a new node
func (b *Backend) Add(conn net.Conn, peerID string) (protoCommon.ProtocolHandler, error) {
	if len(peerID) > 10 {
		peerID = peerID[0:10]
	}

	// use handler to create the connection

	status, err := b.GetStatus()
	if err != nil {
		return nil, err
	}

	logger := b.logger.Named(fmt.Sprintf("peer-%s", peerID))

	proto := NewEthereumProtocol(peerID, logger, conn, b.blockchain)
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
	b.logger.Trace("add node", "id", peerID)

	return proto, nil
}

func (b *Backend) Dequeue() *Job {
	job, err := b.queue.Dequeue()
	if err != nil {
		fmt.Printf("Failed to dequeue: %v\n", err)
	}
	return job
}

func (b *Backend) Deliver(context string, id uint32, data interface{}, err error) {
	b.deliverLock.Lock()
	defer b.deliverLock.Unlock()

	if err != nil {
		// TODO, we need to set here the thing that was not deliver as waiting to be dequeued again
		fmt.Printf("=> Failed to deliver (%d): %v\n", id, err)
		if err := b.queue.updateFailedElem(id, context); err != nil {
			fmt.Printf("Could not be updated: %v\n", err)
		}
		return
	}

	switch obj := data.(type) {
	case []*types.Header:
		b.logger.Trace("deliver headers", "count", len(obj))

		// fmt.Printf("deliver headers %d: %d\n", id, len(obj))
		if err := b.queue.deliverHeaders(id, obj); err != nil {
			panic(fmt.Errorf("Failed to deliver headers (%d): %v", id, err))
		}

	case []*types.Body:
		b.logger.Trace("deliver bodies", "count", len(obj))

		// fmt.Printf("deliver bodies %d: %d\n", id, len(obj))
		if err := b.queue.deliverBodies(id, obj); err != nil {
			panic(fmt.Errorf("Failed to deliver bodies (%d): %v", id, err))
		}

	case [][]*types.Receipt:
		b.logger.Trace("deliver receipts", "count", len(obj))

		// fmt.Printf("deliver receipts %d: %d\n", id, len(obj))
		if err := b.queue.deliverReceipts(id, obj); err != nil {
			panic(fmt.Errorf("Failed to deliver receipts (%d): %v", id, err))
		}

	default:
		panic(data)
	}

	if n := b.queue.NumOfCompletedBatches(); n == b.last {
		b.counter++
	} else {
		b.last = n
		b.counter = 0
	}

	if b.counter == 1000 {
		b.queue.printQueue()
		// TODO, slots in the queue can be parsed again if they are taking too long to query by some peers
		// We should limit the number of elements on the fly at any time
		panic("Some of the data has been pending for too long!")
	}

	if b.queue.NumOfCompletedBatches() >= 1 { // force to commit data every time
		data := b.queue.FetchCompletedData()

		// The data is ready to be committed
		b.commitCh <- data
	}
}

func (b *Backend) commitData() {
	for {
		select {
		case data := <-b.commitCh:

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

					panic("Unhandled error")
				}

				h, _ := b.blockchain.Header()
				b.logger.Info("new header number", "num", h.Number.Uint64())
			}
		}
	}
}

// getSeq returns the next sequence number in a safe manner
func (b *Backend) getSeq() uint64 {
	return atomic.AddUint64(&b.seq, 1)
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
func (b *Backend) FindCommonAncestor(peer *Ethereum, height *types.Header) (*types.Header, *types.Header, error) {
	// Binary search, TODO, works but it may take a lot of time

	h, _ := b.blockchain.Header()

	min := 0 // genesis
	max := int(h.Number.Uint64())

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

func newWorkersHeap() *workersHeap {
	return &workersHeap{
		index: map[string]*worker{},
		heap:  workersHeapImpl{},
	}
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
	delete(w.index, id)
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

// TODO, create a taskRunner interface to do task cleanup tests

type task struct {
	seq      uint64
	job      *Job
	peerID   string
	proto    *Ethereum
	backend  *Backend
	cancelFn context.CancelFunc
}

func (t *task) Run() bool {
	id := t.peerID
	ctx, cancel := context.WithCancel(context.Background())
	t.cancelFn = cancel

	var data interface{}
	var err error
	var context string

	conn := t.proto

	switch job := t.job.payload.(type) {
	case *HeadersJob:
		// fmt.Printf("SYNC HEADERS (%s): %d %d\n", id, job.block, job.count)

		t.backend.logger.Trace("sync headers", "id", id, "from", job.block, "count", job.count)

		data, err = conn.RequestHeadersRangeSync(ctx, job.block, job.count)
		context = "headers"
	case *BodiesJob:
		// fmt.Printf("SYNC BODIES (%s): %d\n", id, len(job.hashes))

		t.backend.logger.Trace("sync bodies", "id", id, "num", len(job.hashes))

		data, err = conn.RequestBodiesSync(ctx, job.hash, job.hashes)
		context = "bodies"
	case *ReceiptsJob:
		// fmt.Printf("SYNC RECEIPTS (%s): %d\n", id, len(job.hashes))

		t.backend.logger.Trace("sync receipts", "id", id, "num", len(job.hashes))

		data, err = conn.RequestReceiptsSync(ctx, job.hash, job.hashes)
		context = "receipts"
	}

	// set the error to the context content
	if ctx.Err() != nil {
		// Do nothing, its an old task
		return false
	}

	if err != nil {
		if err.Error() == "session closed" {
			// remove the peer from the list of peers if the session has been closed
			return true
		}
	}

	t.backend.Deliver(context, t.job.id, data, err)
	return false
}

func (t *task) Close() {
	t.cancelFn()
}
