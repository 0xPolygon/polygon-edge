package ethereum

import (
	"container/heap"
	"context"
	"fmt"
	"math"
	"math/big"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/fastrlp"
	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/minimal"
	"github.com/umbracle/minimal/sealer"

	"sync"

	"github.com/umbracle/minimal/network"

	"github.com/umbracle/minimal/protocol"
	"github.com/umbracle/minimal/types"
)

// Backend is the ethereum backend
type Backend struct {
	NetworkID  uint64
	minimal    *minimal.Minimal
	blockchain *blockchain.Blockchain
	logger     hclog.Logger
	peers      map[string]*Ethereum
	peersLock  sync.Mutex
	heap       *workersHeap

	wakeCh chan struct{}
	taskCh chan struct{}

	notifyCh    chan struct{} // notify for a possible new head
	notifyBlock chan uint64

	skeleton *Queue3

	syncing uint64

	// watcher parts
	target *Ethereum
	wqueue chan *types.Block

	closeCh chan struct{}
}

func Factory(ctx context.Context, logger hclog.Logger, m interface{}, config map[string]interface{}) (protocol.Backend, error) {
	minimal := m.(*minimal.Minimal)
	return NewBackend(minimal, logger, minimal.Blockchain)
}

// NewBackend creates a new ethereum backend
func NewBackend(minimal *minimal.Minimal, logger hclog.Logger, blockchain *blockchain.Blockchain) (*Backend, error) {
	b := &Backend{
		logger:     logger,
		minimal:    minimal,
		peers:      map[string]*Ethereum{},
		peersLock:  sync.Mutex{},
		blockchain: blockchain,
		heap:       newWorkersHeap(),
		wakeCh:     make(chan struct{}, 10),
		taskCh:     make(chan struct{}, maxConcurrentTasks),
		notifyCh:   make(chan struct{}, 1),
		syncing:    0,
		skeleton: &Queue3{
			downloadBodies:   true,
			downloadReceipts: true,
			completed:        false,
			advanceCh:        make(chan struct{}, 1),
			doneCh:           make(chan struct{}, 1),
		},
		target:  nil,
		wqueue:  make(chan *types.Block, 1),
		closeCh: make(chan struct{}, 1),
	}

	b.skeleton.Wait()

	//b.syncer = newSyncer(b)
	//b.syncer.run()

	header, ok := blockchain.Header()
	if !ok {
		return nil, fmt.Errorf("header not found")
	}

	if minimal != nil {
		b.NetworkID = uint64(minimal.Chain().Params.ChainID)
	} else {
		b.NetworkID = 1
	}

	logger.Info("Header", "num", header.Number, "hash", header.Hash.String())

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
var ETH63 = network.ProtocolSpec{
	Name:    "eth",
	Version: 63,
	Length:  17,
}

// Protocols implements the protocol interface
func (b *Backend) Protocols() []*network.Protocol {
	return []*network.Protocol{
		&network.Protocol{
			Spec:      ETH63,
			HandlerFn: b.Add,
		},
	}
}

type syncStatus uint64

const (
	notSyncing syncStatus = 0
	candidate             = 1
	syncing               = 2
)

func (s syncStatus) String() string {
	return strconv.Itoa(int(s))
}

func (b *Backend) getSyncing() syncStatus {
	return syncStatus(atomic.LoadUint64(&b.syncing))
}

func (b *Backend) setSyncing(i syncStatus) {
	atomic.StoreUint64(&b.syncing, uint64(i))
}

// Run implements the protocol interface
func (b *Backend) Run() {
	go b.runSync()
	go b.syncHeader()
	go b.runTasks()
	go b.runWatcher()
}

const maxUncleLen = 7

func (b *Backend) runWatcher() {
	for {
		select {
		case block := <-b.wqueue:
			// get the current header
			header, _ := b.blockchain.Header()
			num := header.Number

			if block.Number() > num {
				// future block
				continue
			}
			if num-maxUncleLen > block.Number() {
				// past block
				continue
			}

			if err := b.blockchain.WriteBlocks([]*types.Block{block}); err != nil {
				fmt.Printf("Err: %v\n", err)
			}
		}
	}
}

func (b *Backend) syncHeader() {
	for {
		select {
		case <-b.notifyCh:
			if status := b.getSyncing(); status == notSyncing {
				best := b.bestPeer()
				if best != nil {
					b.setSyncing(candidate)
					go func() {
						if err := b.syncTarget(best); err != nil {
							fmt.Printf("Stop syncing: %v\n", err)
						}
						b.setSyncing(notSyncing)
					}()
				}
			}
		}
	}
}

const numSlots = 2

func (b *Backend) syncTarget(target *Ethereum) error {
	fmt.Printf("Sync target: %s\n", target.peerID)

	// get the ancestor
	height, err := target.fetchHeight2()
	if err != nil {
		return fmt.Errorf("failed to request height: %v", err)
	}

	// get common ancestor
	ancestor, err := b.FindCommonAncestor(target, height)
	if err != nil {
		return fmt.Errorf("failed to get common ancestor: %v", err)
	}

	// Ancestor query was correct, select this peer for the syncing
	b.setSyncing(syncing)

START:
	// origin is the start position to sync
	origin := ancestor.Number + 1

	// request skeleton
	headers, err := target.requestHeaderByNumber2(origin, nil, numSlots, skeletonSize-1, false)
	if err != nil {
		return fmt.Errorf("failed to request skeleton: %v", err)
	}

	// the point is taht we should have at least one value

	// value in the last header
	last := headers[len(headers)-1]
	lastValue := minUint64(last.Number+skeletonSize-1, height.Number)
	lastSlotNum := lastValue - last.Number

	// TODO, add some mechanism to stop this syncing if there are no more headers to request
	// scenario in the top of the head

	if err := b.skeleton.AddSkeleton(headers, lastSlotNum); err != nil {
		return err
	}

	var num uint64

	select {
	case <-b.closeCh:
		// syncer is stopping

		b.skeleton.Wait()
		b.skeleton.reset()

		return nil
	case <-b.skeleton.doneCh:
		ancestor = b.skeleton.Head()

		if ancestor.Number == height.Number {
			// exit
			num = ancestor.Number
		} else {
			b.skeleton.reset()
			goto START
		}
	}

	fmt.Printf("Sync done: %d\n", num)

	return nil
}

func (b *Backend) runTasks() {
	for i := 0; i < 10; i++ {
		go b.runTask(strconv.Itoa(i))
	}
}

func (b *Backend) runTask(id string) {
	// This part will eventually go into the full_sync.go file
	var req Request

	// wait for the skeleton to be ready
	// This has to change later in case that we have to update the skeleton
	// and the tasks have to wait for a moment
	// <-s.skeletonCh

	// fmt.Printf("Task %s started\n", id)

	for {
		w := b.peek()

		for !b.skeleton.GetJob(&req) { // Wait till there is a job
			time.Sleep(1 * time.Second)
		}

		fmt.Printf("Task %s. Worker %s. ReqType: %s (%d). Slot (%d)\n", id, w.id, req.typ.String(), req.count, req.slot)

		err := w.proto.DoRequest(&req, b.skeleton)

		fmt.Printf("Task %s. Worker %s. %v\n", id, w.id, err)

		if err != nil {

			fmt.Printf("Reassign %s. Worker %s. ReqType: %s (%d). Slot (%d)\n", id, w.id, req.typ.String(), req.count, req.slot)

			b.skeleton.ReassignJob(&req)
		}

		// wait and crash
		if err != nil {
			fmt.Println("===============================================================>")
		}

		b.releaseWorker(w.id, err != nil)
	}
}

func (b *Backend) announceNewBlock(e *Ethereum, p *fastrlp.Parser, v *fastrlp.Value) error {
	/*
		elems, err := v.GetElems()
		if err != nil {
			return err
		}
		if len(elems) != 2 {
			return fmt.Errorf("bad")
		}

		// difficulty in elems.0
		diff := new(big.Int)
		if err := elems[0].GetBigInt(diff); err != nil {
			return err
		}

		// block in elems.1
		subElems, err := elems[1].GetElems()
		if err != nil {
			return err
		}
		if len(subElems) != 3 {
			return fmt.Errorf("bad.2")
		}
	*/
	return nil
}

func (b *Backend) announceNewBlocksHashes(e *Ethereum, v *fastrlp.Value) error {
	elems, _ := v.GetElems()

	for _, item := range elems {
		tuple, _ := item.GetElems()
		hash := types.BytesToHash(tuple[0].Raw())

		block, err := e.fetchBlock(hash)
		if err != nil {
			continue
		}

		select {
		case b.wqueue <- block:
		default:
		}
	}

	return nil
}

func (b *Backend) bestPeer() *Ethereum {
	b.peersLock.Lock()
	defer b.peersLock.Unlock()

	var p *Ethereum
	diff, ok := b.blockchain.GetChainTD()
	if !ok {
		return nil
	}

	for _, item := range b.peers {
		if item.HeaderDiff != nil && item.HeaderDiff.Cmp(diff) > 0 {
			diff = item.HeaderDiff
			p = item
		}
	}
	return p
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

func (b *Backend) releaseWorker(peerID string, failed bool) {
	if !failed {
		outstanding, _ := b.heap.Update(peerID, -1)
		if outstanding == maxOutstandingRequests-1 {
			b.notifyAvailablePeer()
		}
	} else {
		// peer has failed, remove it from the list
		b.heap.Remove(peerID)
	}

	// release the task
	b.taskCh <- struct{}{}
}

func (b *Backend) notifyAvailablePeer() {
	select {
	case b.wakeCh <- struct{}{}:
	default:
	}
}

func (b *Backend) WatchMinedBlocks(watch chan *sealer.SealedNotify) {
	for {
		w := <-watch
		// TODO, change with a blockchain listener
		go b.broadcastBlock(w.Block)
	}
}

var defaultArena fastrlp.ArenaPool

func (b *Backend) broadcastBlock(block *types.Block) {
	fmt.Println("== BROADCAST BLOCK ==")

	// total difficulty so far at the parent
	diff, ok := b.blockchain.GetTD(block.ParentHash())
	if !ok {
		// log error
		return
	}

	// total difficulty + the difficulty of the block
	blockDiff := big.NewInt(1).Add(diff, new(big.Int).SetUint64(block.Header.Difficulty))

	ar := defaultArena.Get()

	// send block
	v0 := ar.NewArray()
	v0.Set(ar.NewBigInt(blockDiff))
	v0.Set(block.MarshalWith(ar))

	for _, i := range b.peers {
		if err := i.writeRLP(NewBlockMsg, v0); err != nil {
			fmt.Printf("Failed to send send block to peer %s: %v\n", i.peerID, err)
		}
	}

	// send hashes
	v1 := ar.NewArray()
	tuple := ar.NewArray()
	tuple.Set(ar.NewCopyBytes(block.Hash().Bytes()))
	tuple.Set(ar.NewUint(block.Number()))
	v1.Set(tuple)

	for _, i := range b.peers {
		if err := i.writeRLP(NewBlockHashesMsg, v1); err != nil {
			fmt.Printf("Failed to send hashes to peer %s: %s\n", i.peerID, err)
		}
	}

	defaultArena.Put(ar)
}

// Add is called when we connect to a new node
func (b *Backend) Add(conn net.Conn, peer *network.Peer) (network.ProtocolHandler, error) {
	peerID := peer.PrettyID()

	// use handler to create the connection

	status, err := b.GetStatus()
	if err != nil {
		return nil, err
	}

	logger := b.logger.Named(fmt.Sprintf("peer-%s", peerID))

	proto := NewEthereumProtocol(peer.Session(), peerID, logger, conn, b.blockchain)
	proto.backend = b

	b.peersLock.Lock()
	if _, ok := b.peers[peerID]; ok {
		b.peersLock.Unlock()
		return nil, nil
	}

	b.peers[peerID] = proto
	b.peersLock.Unlock()

	// Start the protocol handle
	if err := proto.Init(status); err != nil {
		return nil, err
	}

	// Only validate for the DAO Fork on Ethereum mainnet
	if b.NetworkID == 1 {
		if err := proto.ValidateDAOBlock(); err != nil {
			fmt.Printf("failed to validate dao: %v\n", err)
			return nil, err
		}
	}

	b.addPeer(peerID, proto)
	b.logger.Trace("add node", "id", peerID)

	select {
	case b.notifyCh <- struct{}{}:
	default:
	}

	return proto, nil
}

func (b *Backend) runSync() {
	for {
		block := b.skeleton.Next()

		b.logger.Info("write block", "hash", block.Hash(), "number", block.Number())

		if err := b.blockchain.WriteBlocks([]*types.Block{block}); err != nil {
			//
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
	td, ok := b.blockchain.GetTD(header.Hash)
	if !ok {
		return nil, fmt.Errorf("header difficulty not found")
	}

	status := &Status{
		ProtocolVersion: 63,
		NetworkID:       b.NetworkID,
		TD:              td,
		CurrentBlock:    header.Hash,
		GenesisBlock:    b.blockchain.Genesis(),
	}
	return status, nil
}

// FindCommonAncestor finds the common ancestor with the peer and the syncer connection
func (b *Backend) FindCommonAncestor(peer *Ethereum, height *types.Header) (*types.Header /* *types.Header, */, error) {
	// Binary search, TODO, works but it may take a lot of time

	h, _ := b.blockchain.Header()

	min := uint64(0) // genesis
	max := h.Number

	if heightNumber := height.Number; max > heightNumber {
		max = heightNumber
	}

	var header *types.Header
	// var ok bool

	// ctx := context.Background()
	for min <= max {
		m := uint64(math.Floor(float64(min+max) / 2))

		found, err := peer.requestHeaderByNumber(m)
		if err != nil && err != errorEmptyQuery {
			return nil, err
		}

		if err == errorEmptyQuery {
			// peer does not have the m peer, search in lower bounds
			max = m - 1
		} else {
			if found.Number != m {
				return nil, fmt.Errorf("header response number not correct, asked %d but retrieved %d", m, header.Number)
			}

			expectedHeader, ok := b.blockchain.GetHeaderByNumber(m)
			if !ok {
				return nil, fmt.Errorf("cannot find the header %d in local chain", m)
			}
			if expectedHeader.Hash == found.Hash {
				header = found
				min = m + 1
			} else {
				if m == 0 {
					return nil, fmt.Errorf("genesis does not match?")
				}
				max = m - 1
			}
		}
	}

	if min == 0 {
		return nil, nil
	}

	return header, nil
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
