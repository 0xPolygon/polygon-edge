package syncer

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/network"
)

var (
	daoBlock            = uint64(1920000)
	daoChallengeTimeout = 5 * time.Second
)

// Blockchain is the reference the syncer needs to connect to the blockchain
type Blockchain interface {
	Header() (*types.Header, error)
	WriteHeaders(headers []*types.Header) error
}

// Job is the syncer job
type Job struct {
	id    uint32
	block uint64
}

// Syncer is the syncer protocol
type Syncer struct {
	NetworkID int

	peers      map[string]*Peer
	blockchain Blockchain

	WorkerPool chan chan Job

	list     *list
	listLock *sync.Mutex
}

// NewSyncer creates a new syncer
func NewSyncer(networkID int, blockchain Blockchain) *Syncer {
	s := &Syncer{
		NetworkID:  networkID,
		peers:      map[string]*Peer{},
		WorkerPool: make(chan chan Job),
		blockchain: blockchain,
		listLock:   &sync.Mutex{},
	}

	return s
}

// AddNode is called when we connect to a new node
func (s *Syncer) AddNode(peer *network.Peer) {
	fmt.Println("----- ADD NODE -----")

	p := NewPeer(peer, s.WorkerPool, s)
	s.peers[peer.ID] = p

	go p.run()
}

// Run is the main entry point
func (s *Syncer) Run() {
	// get the header from the blockchain
	header, err := s.blockchain.Header()
	if err != nil {
		panic(err)
	}

	fmt.Printf("Current header: %d\n", header.Number.Uint64())

	s.list = newList(header.Number.Uint64()+1, 6000000)

	for {
		idle := <-s.WorkerPool

		i := s.getSlot()
		if i == nil {
			panic("its nil")
		}

		fmt.Printf("SYNC: %d\n", i.block)
		idle <- i.ToJob()
	}
}

func (s *Syncer) getSlot() *item {
	s.listLock.Lock()
	defer s.listLock.Unlock()

	return s.list.GetQuerySlot()
}

func (s *Syncer) result(id uint32, headers []*types.Header, err error) {
	s.listLock.Lock()
	defer s.listLock.Unlock()

	var t itemType
	if err != nil {
		t = failed
	} else if len(headers) != 100 {
		t = failed
	} else {
		t = completed
	}

	if err := s.list.UpdateSlot(id, t, headers); err != nil {
		fmt.Printf("FAIL: %v\n", err)
	}

	fmt.Println(s.list.numCompleted())

	// check the number of completed items to commit those values
	// protect with lock
	//if s.list.numCompleted() == 1 {
	// commit values to the storage
	fmt.Println("## COMMIT ##")

	hh := s.list.commitData()
	if len(hh) != 0 {
		fmt.Println("GOING IN")
		if err := s.blockchain.WriteHeaders(hh); err != nil {
			fmt.Printf("FAILED TO COMMIT: %v\n", err)
		}
		fmt.Println("DONE")
	}

	//}
}

/*
// TODO
func (s *Syncer) checkDAOHardFork(eth *ethereum.Ethereum) error {
	if s.NetworkID == 1 {
		ack := make(chan network.AckMessage, 1)
		eth.Conn().SetHandler(ethereum.BlockHeadersMsg, ack, daoChallengeTimeout)

		// check the DAO block
		if err := eth.RequestHeadersByNumber(daoBlock, 1, 0, false); err != nil {
			return err
		}

		resp := <-ack
		if resp.Complete {
			var headers []*types.Header
			if err := rlp.DecodeBytes(resp.Payload, &headers); err != nil {
				return err
			}

			// TODO. check that daoblock is correct

		} else {
			return fmt.Errorf("timeout")
		}
	}

	return nil
}
*/

// -- linked list

const (
	maxElements = 100
)

type list struct {
	front, back *item
	seq         uint32
	lock        *sync.Mutex
}

func newList(head, last uint64) *list {
	l := &list{seq: 0, lock: &sync.Mutex{}}

	l.front = l.newItem(head)
	l.back = l.newItem(last)

	l.front.next = l.back
	l.back.prev = l.front

	return l
}

func (l *list) UpdateSlot(id uint32, t itemType, headers []*types.Header) error {
	l.lock.Lock()
	defer l.lock.Unlock()

	item := l.findSlot(id)
	if item == nil {
		return fmt.Errorf("not found item %d", id)
	}

	if item.t != pending {
		return fmt.Errorf("The state of the knot should be pending")
	}

	item.t = t
	if item.t == completed {
		item.headers = headers
	}

	return nil
}

func (l *list) findSlot(id uint32) *item {
	elem := l.front
	for elem != nil {
		if elem.id == id {
			return elem
		}
		elem = elem.next
	}
	return nil
}

func (l *list) GetQuerySlot() *item {
	l.lock.Lock()
	defer l.lock.Unlock()

	elem := l.nextSlot()
	if elem == nil {
		return nil
	}

	if elem.Len() <= maxElements {
		elem.t = pending
		return elem
	}

	// split the item
	i := l.newItem(elem.block + maxElements)

	i.prev = elem
	i.next = elem.next

	elem.next = i
	elem.t = pending

	return elem
}

func (l *list) numCompleted() int {
	n := 0
	elem := l.front
	for elem != nil {
		if elem.t != completed {
			break
		}
		n++
		elem = elem.next
	}
	return n
}

func (l *list) nextSlot() *item {
	return l.nextSlotFromItem(l.front)
}

func (l *list) nextSlotFromItem(elem *item) *item {
	for elem != nil {
		if elem.t == failed || elem.t == empty {
			return elem
		}
		elem = elem.next
	}
	return nil
}

func (l *list) newItem(block uint64) *item {
	return &item{id: l.nextSeqNo(), block: block, t: empty}
}

func (l *list) nextSeqNo() uint32 {
	return atomic.AddUint32(&l.seq, 1)
}

func (l *list) commitData() []*types.Header {
	l.lock.Lock()
	defer l.lock.Unlock()

	headers := []*types.Header{}

	// remove the completed values and return all the headers
	elem := l.front
	for elem != nil {
		if elem.t != completed {
			break
		}
		headers = append(headers, elem.headers...)
		elem = elem.next
	}

	l.front = elem
	return headers
}

type itemType int

const (
	failed itemType = iota
	completed
	pending
	empty
)

type item struct {
	id         uint32
	block      uint64
	prev, next *item
	t          itemType
	headers    []*types.Header
}

func (i *item) Len() uint64 {
	return i.next.block - i.block
}

func (i *item) ToJob() Job {
	return Job{block: i.block, id: i.id}
}
