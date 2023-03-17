package service

import (
	"container/heap"
	"sync"

	"github.com/0xPolygon/polygon-edge/types"
)

// AAPool defines the interface for a pool of Account Abstraction (AA) transactions
type AAPool interface {
	// Push adds an AA state transaction to the pool, associating it with the given account ID
	Push(*AAStateTransaction)
	// Pop removes the next transaction from the pool and returns it
	Pop() *AAStateTransaction
	// Update performs pool update for given address if needed
	Update(types.Address)
	// Init initializes the pool with a set of existing AA transactions. Used on client startup
	Init([]*AAStateTransaction)
	// Len returns number of items in pool
	Len() int
}

var _ AAPool = (*aaPool)(nil)

type aaPool struct {
	// mutex is used for synchronization
	mutex sync.Mutex
	// perAddress keeps for each address binary heap where txs are sorted by nonce
	perAddressNonceHeap map[types.Address]*aaPoolNonceHeap
	// perAddressItem keeps item from timeHeap for each address
	perAddressItem map[types.Address]*aaPoolTimeHeapItem
	// timeHeap is a binary heap where txs are sorted by time
	// only one tx from same address can be inside timeHeap
	// that tx corresponds to the first one from perAddressNonceHeap[address]
	// this allows for efficient retrieval of the oldest tx with lowest nonce in the entire pool
	timeHeap aaPoolTimeHeap
	// count is count of all txs
	count int
}

func NewAAPool() *aaPool {
	timeHeap := aaPoolTimeHeap{}
	heap.Init(&timeHeap)

	return &aaPool{
		perAddressNonceHeap: map[types.Address]*aaPoolNonceHeap{},
		perAddressItem:      map[types.Address]*aaPoolTimeHeapItem{},
		timeHeap:            timeHeap,
		count:               0,
	}
}

func (p *aaPool) Len() int {
	return p.count
}

func (p *aaPool) Push(stateTx *AAStateTransaction) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	address := stateTx.Tx.Transaction.From

	nonceHeap, exists := p.perAddressNonceHeap[address]
	if !exists {
		// this is the first stateTx from address - simple update of binary heaps and perAddressItem map
		timeHeapItem := &aaPoolTimeHeapItem{stateTx: stateTx}
		p.perAddressNonceHeap[address] = &aaPoolNonceHeap{stateTx}
		p.perAddressItem[address] = timeHeapItem
		heap.Push(&p.timeHeap, timeHeapItem)
	} else {
		heap.Push(nonceHeap, stateTx)

		if prevTimeHeapItem, exists := p.perAddressItem[address]; exists {
			// timeHeap already contains item from address - get one with the smallest nonce and update position
			prevTimeHeapItem.stateTx = nonceHeap.Peek()
			heap.Fix(&p.timeHeap, prevTimeHeapItem.index)
		} else {
			// timeHeap does not contain item from address - add one with the smallest nonce to the timeHeap
			timeHeapItem := &aaPoolTimeHeapItem{stateTx: nonceHeap.Peek()}
			p.perAddressItem[address] = timeHeapItem
			heap.Push(&p.timeHeap, timeHeapItem)
		}
	}

	p.count++
}

func (p *aaPool) Pop() *AAStateTransaction {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.timeHeap.Len() == 0 {
		return nil
	}

	timeHeapItem := heap.Pop(&p.timeHeap).(*aaPoolTimeHeapItem) //nolint
	address := timeHeapItem.stateTx.Tx.Transaction.From
	//  also remove first item from perAddress
	_ = heap.Pop(p.perAddressNonceHeap[address])
	// delete item from perAddressItem. timeHeap will not contain item from address until Update is called
	delete(p.perAddressItem, address)

	p.count--

	return timeHeapItem.stateTx
}

func (p *aaPool) Update(address types.Address) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.perAddressItem[address] != nil { // perAddressItem is already populated
		return
	}

	nonceHeap := p.perAddressNonceHeap[address]

	if nonceHeap != nil && nonceHeap.Len() > 0 {
		timeHeapItem := &aaPoolTimeHeapItem{stateTx: nonceHeap.Peek()}
		heap.Push(&p.timeHeap, timeHeapItem)     // push first item from nonce heap to the time heap
		p.perAddressItem[address] = timeHeapItem // update perAddressItem
	} else {
		// delete binary heap for address if binary heap is empty
		delete(p.perAddressNonceHeap, address)
	}
}

func (p *aaPool) Init(txs []*AAStateTransaction) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.perAddressNonceHeap = map[types.Address]*aaPoolNonceHeap{}
	p.perAddressItem = map[types.Address]*aaPoolTimeHeapItem{}
	p.timeHeap = aaPoolTimeHeap{}
	p.count = len(txs)

	for _, stateTx := range txs {
		address := stateTx.Tx.Transaction.From

		// put each stateTx into appropriate nonceHeap binary heap
		if nonceHeap, exists := p.perAddressNonceHeap[address]; exists {
			*nonceHeap = append(*nonceHeap, stateTx)
		} else {
			p.perAddressNonceHeap[address] = &aaPoolNonceHeap{stateTx}
		}
	}

	for _, nonceHeap := range p.perAddressNonceHeap {
		heap.Init(nonceHeap) // init each nonceHeap binary heap

		// populate timeHeap with all the first items from the each nonceHeap
		p.timeHeap = append(p.timeHeap, &aaPoolTimeHeapItem{
			stateTx: nonceHeap.Peek(),
			index:   len(p.timeHeap),
		})
	}

	heap.Init(&p.timeHeap) // init timeHeap binary heap

	// update perAddressItem
	for _, timeHeapItem := range p.timeHeap {
		address := timeHeapItem.stateTx.Tx.Transaction.From

		p.perAddressItem[address] = timeHeapItem
	}
}

type aaPoolTimeHeapItem struct {
	stateTx *AAStateTransaction
	index   int
}

type aaPoolTimeHeap []*aaPoolTimeHeapItem

func (h aaPoolTimeHeap) Len() int { return len(h) }

func (h aaPoolTimeHeap) Less(i, j int) bool {
	return h[i].stateTx.Time < h[j].stateTx.Time
}

func (h aaPoolTimeHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index, h[j].index = i, j
}

func (h *aaPoolTimeHeap) Push(x interface{}) {
	item := x.(*aaPoolTimeHeapItem) //nolint
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *aaPoolTimeHeap) Pop() interface{} {
	n := len(*h) - 1
	x := (*h)[n]
	(*h)[n] = nil
	*h = (*h)[0:n]

	return x
}

type aaPoolNonceHeap []*AAStateTransaction

func (h aaPoolNonceHeap) Len() int { return len(h) }

func (h aaPoolNonceHeap) Less(i, j int) bool {
	return h[i].Tx.Transaction.Nonce < h[j].Tx.Transaction.Nonce
}

func (h aaPoolNonceHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *aaPoolNonceHeap) Push(x interface{}) {
	*h = append(*h, x.(*AAStateTransaction)) //nolint
}

func (h *aaPoolNonceHeap) Pop() interface{} {
	n := len(*h) - 1
	x := (*h)[n]
	(*h)[n] = nil
	*h = (*h)[0:n]

	return x
}

func (h *aaPoolNonceHeap) Peek() *AAStateTransaction {
	return (*h)[0]
}
