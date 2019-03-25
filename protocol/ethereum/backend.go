package ethereum

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"net"

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
	GetTD(hash common.Hash) *big.Int
	WriteHeaders(headers []*types.Header) error
	WriteBlocks(blocks []*types.Block) error
	CommitBodies(headers []common.Hash, bodies []*types.Body) error
	CommitReceipts(headers []common.Hash, receipts []types.Receipts) error
	GetHeaderByHash(hash common.Hash) *types.Header
	GetHeaderByNumber(n *big.Int) *types.Header
	GetReceiptsByHash(hash common.Hash) types.Receipts
	GetBodyByHash(hash common.Hash) *types.Body
}

type Peer struct {
	pretty   string
	id       string
	conn     *Ethereum
	peerConn *PeerConnection
	active   bool
	failed   int
	pending  int
}

// TODO, Is it possible to have differents versions of the same protocol
// running at the same time?

func newPeer(id string, conn *Ethereum, peerConn *PeerConnection) *Peer {
	return &Peer{
		id:       id,
		active:   true,
		pretty:   id,
		conn:     conn,
		peerConn: peerConn,
	}
}

// Backend is the ethereum backend
type Backend struct {
	NetworkID  uint64
	config     *Config
	minimal    *minimal.Minimal
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

	stopFn context.CancelFunc
}

func Factory(ctx context.Context, m interface{}) (protocol.Backend, error) {
	minimal := m.(*minimal.Minimal)
	return NewBackend(minimal, 1, minimal.Blockchain, DefaultConfig())
}

// NewBackend creates a new ethereum backend
func NewBackend(minimal *minimal.Minimal, networkID uint64, blockchain Blockchain, config *Config) (*Backend, error) {
	b := &Backend{
		config:      config,
		minimal:     minimal,
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

	if minimal != nil {
		go b.WatchMinedBlocks(minimal.Sealer.SealedCh)
	}

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

func (b *Backend) WatchMinedBlocks(watch chan *sealer.SealedNotify) {
	for {
		w := <-watch

		fmt.Println("-- watch --")
		fmt.Println(w)

		// If we manage to create the headerchain data structure, the
		// sealer is just another fork, in this case a local fork.

		// Notify that a new block has been sealed
		// the blockchain has been update the most likely
		// What if we dont get the notification from the sealer but
		// from the blockchain? Every time there is an update
		// so that we dont have communication between the sealer
		// and the backend and just with the blockchain. The same way that
		// sealer talks with the blockchain to know about updates!

		// Not sure if it has to recompute some of the data.
		// This is only called when the sealer is activated so it does not need
		// to be in 'syncer' mode but in a more limited 'watch' mode.

		// For now just broadcast all the nodes that arrve here.
		go b.broadcast(w.Block)
	}
}

func (b *Backend) broadcast(block *types.Block) {
	// total difficulty so far at the parent
	diff := b.blockchain.GetTD(block.ParentHash())

	// total difficulty + the difficulty of the block
	blockDiff := big.NewInt(1).Add(diff, block.Difficulty())

	for _, i := range b.peers {
		i.conn.SendNewBlock(block, blockDiff)
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

		fmt.Println("BLOCK UPDATE FROM THE INTERNAL CALLS")
		fmt.Println(w.Block)
		fmt.Println(w.Diff)
		fmt.Println(w.Peer)

		go b.notifyNewData(w)
	}
}

// TODO, sendBlocks for the notification does not send the full difficulty

func (b *Backend) notifyNewData(w *NotifyMsg) {
	fmt.Println(w)

	fmt.Println("-- peer id --")
	fmt.Println(w.Peer.id)

	peerEth := b.peers[w.Peer.id].conn

	// check if difficulty is higher than ours
	// it its higher do the check about the header and ancestor

	// find data about the peer
	header, err := b.fetchHeight(peerEth)
	if err != nil {
		fmt.Printf("ERR: fetch height failed: %v\n", err)
		return
	}

	fmt.Printf("Heigth: %d\n", header.Number.Uint64())

	// TODO, do this only if the difficulty is higher
	ancestor, forkHeader, err := b.FindCommonAncestor(peerEth)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Ancestor: %d\n", ancestor.Number.Uint64())

	fmt.Printf("========================> Ancestor: %d\n", ancestor.Number.Uint64())

	// This header will be local once we do the headerChain object
	ourHeader := b.blockchain.Header() // keep this header locally? maybe return the last header from blockchain

	// In go-ethereum this part is on the handler, we have it here to combine it with the whole sync process
	// peerEth.HeaderDiff is the total difficulty of his chain, we need to check the difficulty of our current chain
	// at the block number.

	td := b.blockchain.GetTD(ourHeader.Hash())

	// check that the difficulty is higher than ours
	if peerEth.HeaderDiff.Cmp(td) < 0 {
		fmt.Printf("====================> Difficulty %s is lower than ours %s, skip it\n", peerEth.HeaderDiff.String(), td.String())
	} else {
		// We discovered a new difficulty, start to query this one.

		fmt.Println("Difficulty higher than ours")
		fmt.Println(peerEth.HeaderDiff.Int64())
		fmt.Println(td.Int64())

		// IMPORTANT NOTE: if we are notified with a block
		// that is maybe his last block sealed, then, ancestor should not be common by any means
		// ancestor should be one before the advertised one.

		if forkHeader == nil {
			fmt.Println("- on the tip -")
			// we are at the tip of the chain. skip
			// we will be awaked later on with the block update
			// difference of one block.
			return
		}

		fmt.Printf("Fork giveaway: %s (%d)\n", forkHeader.Hash().String(), forkHeader.Number.Uint64())

		fmt.Println("-- fork parent --")
		fmt.Println(forkHeader.ParentHash.String())

		fmt.Println("-- ancestor (this should be fork parent) --")
		fmt.Println(ancestor.Hash().String())
		fmt.Println(ancestor.Number.Uint64())

		// TODO, it needs a better context of the situation, can be solved with the forks
		// Notify the block to the sealer
		b.minimal.Sealer.NotifyBlock(w.Block)

		// hopefully, with headerChain it will be easier to mark this fork,
		// we write this first fork to give a small pointer to the new path.
		// We need to write block otherwise the state will not be there.

		forkBlock := []*types.Block{
			types.NewBlockWithHeader(forkHeader),
		}
		if err := b.blockchain.WriteBlocks(forkBlock); err != nil {
			panic(err)
		}

		/*
			// write the forkBlock, this creates a fork in the blockchain
			if err := b.blockchain.WriteHeaders([]*types.Header{forkHeader}); err != nil {
				panic(err)
			}
		*/

		// Once everyone is trying to download
		// Different peers may be in different forks
		// so some of the peers may return incorrect data

		// Change the queue to start to download from the ancestor position
		b.queue = newQueue()
		b.queue.front = b.queue.newItem(ancestor.Number.Uint64() + 1)
		b.queue.head = ancestor.Hash()

		// Wait, are we going to run this every time there is a node update from the upper layer?
		// I mean the find ancestor one. no, you dont have to run the ancestor then.
		// geth? There must be a second channel.
		// The point is to check if ancestor is on our current fork or not, or if we have to change the fork
		// If ancestor is in our current fork, we hve to stay, otherwise change the fork
		// Maybe have the blockchain check where that ancestor belongs

		// Update the chain last header
		b.updateChain(header.Number.Uint64())

		// TODO: Reset peer connections to avoid receive bad values and start the whole process
		// Important to do it after the queue is defined
		for _, p := range b.peers {
			p.peerConn.Reset()
		}
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

	peerConn := &PeerConnection{
		conn:   proto,
		sched:  b,
		peerID: peerID,
	}
	// peerConn.Run()

	p := newPeer(peerID, proto, peerConn)
	b.peers[peerID] = p

	proto.peer = p

	// wake up some task
	// s.wakeUp()

	b.watchBlock(proto.Watch())

	// notifiy this node data
	b.notifyNewData(&NotifyMsg{
		Peer: p,
		Diff: proto.HeaderDiff,
	})

	// go s.runPeer(p)
	return nil
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

	if b.queue.NumOfCompletedBatches() >= 1 { // force to commit data every time
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

			// Write blocks and commit new data
			if err := b.blockchain.WriteBlocks(blocks); err != nil {
				fmt.Printf("Failed to write headers batch: %v", err)

				first, last := elem.headers[0], elem.headers[len(elem.headers)-1]

				fmt.Printf("Error at step: %d\n", indx)
				fmt.Printf("First block we have is %s (%s) %s (%s)\n", first.Hash().String(), first.Number.String(), last.Hash().String(), last.Number.String())

				panic("")
				return
			}

			fmt.Printf("New header number: %d\n", b.blockchain.Header().Number.Uint64())

			/*
				if err := b.blockchain.CommitBodies(elem.GetBodiesHashes(), elem.bodies); err != nil {
					fmt.Printf("Failed to write bodies: %v", err)
					panic("")
				}
				if err := b.blockchain.CommitReceipts(elem.GetReceiptsHashes(), elem.receipts); err != nil {
					fmt.Printf("Failed to write receipts: %v", err)
					panic("")
				}
			*/
		}
	}
}

// GetStatus returns the current ethereum status
func (b *Backend) GetStatus() (*Status, error) {
	header := b.blockchain.Header()

	// We transmit the total difficulty of our current chain
	td := b.blockchain.GetTD(header.Hash())

	status := &Status{
		ProtocolVersion: 63,
		NetworkID:       b.NetworkID,
		TD:              td,
		CurrentBlock:    header.Hash(),
		GenesisBlock:    b.blockchain.Genesis().Hash(),
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

// FindCommonAncestor finds the common ancestor with the peer and the syncer connection
func (b *Backend) FindCommonAncestor(peer *Ethereum) (*types.Header, *types.Header, error) {
	// Binary search, TODO, works but it may take a lot of time

	min := 0 // genesis
	max := int(b.blockchain.Header().Number.Uint64())

	/*
		fmt.Println("-- local max --")
		fmt.Println(max)
	*/

	height, err := b.fetchHeight(peer)
	if err != nil {
		return nil, nil, err
	}
	if heightNumber := int(height.Number.Uint64()); max > heightNumber {
		max = heightNumber
	}

	/*
		fmt.Println("-- remote height --")
		fmt.Println(height)

		fmt.Println("-- max --")
		fmt.Println(max)
	*/

	var header *types.Header
	var ok bool

	ctx := context.Background()
	for min <= max {
		m := uint64(math.Floor(float64(min+max) / 2))

		fmt.Println("-- point --")
		fmt.Println(m)

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

			expectedHeader := b.blockchain.GetHeaderByNumber(big.NewInt(int64(m)))
			if expectedHeader == nil {
				return nil, nil, fmt.Errorf("cannot find the header in local chain")
			}

			if expectedHeader.Hash() == found.Hash() {
				// fmt.Println("-- match --")

				/*
					fmt.Printf("Match: %d\n", m)
					fmt.Println(found.Hash().String())

					ff, ok, err := peer.RequestHeaderSync(ctx, m+1)
					if err != nil {
						panic(err)
					}
					if ok {
						fmt.Printf("Fork: %s\n", ff.ParentHash.String())
					}
				*/

				header = found
				min = int(m + 1)
			} else {
				fmt.Println("-- dont match --")

				max = int(m - 1)
			}
		}
	}

	if min == 0 {
		return nil, nil, nil
	}

	/*
		fmt.Println("-- final header number --")
		fmt.Println(header.Number.Uint64())
	*/

	// Get next element, that would be the index of the fork
	if height.Number.Uint64() == header.Number.Uint64() {
		fmt.Println("- done -")
		return header, nil, nil
	}

	fork, ok, err := peer.RequestHeaderSync(ctx, header.Number.Uint64()+1)
	if err != nil {
		return nil, nil, err
	}
	if !ok { // Can this happen if we check the height point?
		return nil, nil, fmt.Errorf("fork point not found")
	}

	fmt.Println("-- common ancestor info --")

	fmt.Printf("Ancestor: hash(%s), parent(%s), number(%d)\n", header.Hash().String(), header.ParentHash.String(), header.Number.Uint64())
	fmt.Printf("Fork: hash(%s), parent(%s), number(%d)\n", fork.Hash().String(), fork.ParentHash.String(), fork.Number.Uint64())

	return header, fork, nil
}

func (b *Backend) FetchHeight(peer *Ethereum) (*types.Header, error) {
	return b.fetchHeight(peer)
}

// fetchHeight returns the header of the head hash of the peer
func (b *Backend) fetchHeight(peer *Ethereum) (*types.Header, error) {
	head := peer.Header()

	header, err := peer.RequestHeaderByHashSync(context.Background(), head)
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
