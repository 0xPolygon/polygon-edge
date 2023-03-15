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
	perAddress map[types.Address]aaPoolAddressData
	// timeHeap is a binary heap where txs are sorted by time.
	// The top tx from each key in the perAddress map is placed into this heap.
	// This allows for efficient retrieval of the oldest tx in the entire pool.
	timeHeap aaPoolTimeHeap
	// count is count of all txs
	count int
}

func NewAAPool() *aaPool {
	timeHeap := aaPoolTimeHeap{}
	heap.Init(&timeHeap)

	return &aaPool{
		perAddress: make(map[types.Address]aaPoolAddressData),
		timeHeap:   timeHeap,
		count:      0,
	}
}

func (p *aaPool) Len() int {
	return p.count
}

func (p *aaPool) Push(stateTx *AAStateTransaction) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	from := stateTx.Tx.Transaction.From
	timeHeapItem := &aaPoolTimeHeapItem{stateTx: stateTx}

	cont, exists := p.perAddress[from]
	if !exists {
		heap.Push(&p.timeHeap, timeHeapItem)
		p.perAddress[from] = aaPoolAddressData{
			pool: &aaPoolNonceHeap{stateTx},
			item: timeHeapItem,
		}
	} else {
		heap.Push(cont.pool, stateTx)
		cont.item.stateTx = cont.pool.Peek() // new timeHeap item should be first from nonce heap
		heap.Fix(&p.timeHeap, cont.item.index)
		p.perAddress[from] = cont // update map
	}

	p.count++
}

func (p *aaPool) Pop() *AAStateTransaction {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.timeHeap.Len() == 0 {
		return nil
	}

	el := heap.Pop(&p.timeHeap).(*aaPoolTimeHeapItem) //nolint
	cont := p.perAddress[el.stateTx.Tx.Transaction.From]
	_ = heap.Pop(cont.pool) // remove from perAddress also

	if cont.pool.Len() > 0 {
		el := &aaPoolTimeHeapItem{
			stateTx: cont.pool.Peek(),
		}
		// push first from nonce heap to the time heap
		heap.Push(&p.timeHeap, el)
		// update map
		p.perAddress[el.stateTx.Tx.Transaction.From] = aaPoolAddressData{
			item: el,
			pool: cont.pool,
		}
	} else {
		// remove binary heap and item cache from perAddress map
		delete(p.perAddress, el.stateTx.Tx.Transaction.From)
	}

	p.count--

	return el.stateTx
}

func (p *aaPool) Init(txs []*AAStateTransaction) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.perAddress = make(map[types.Address]aaPoolAddressData)
	p.timeHeap = aaPoolTimeHeap{}
	p.count = len(txs)

	for _, tx := range txs {
		if cont, exists := p.perAddress[tx.Tx.Transaction.From]; exists {
			*cont.pool = append(*cont.pool, tx)
		} else {
			p.perAddress[tx.Tx.Transaction.From] = aaPoolAddressData{
				pool: &aaPoolNonceHeap{tx},
			}
		}
	}

	for _, cont := range p.perAddress {
		heap.Init(cont.pool)

		p.timeHeap = append(p.timeHeap, &aaPoolTimeHeapItem{
			stateTx: cont.pool.Peek(),
			index:   len(p.timeHeap),
		})
	}

	heap.Init(&p.timeHeap)

	for _, el := range p.timeHeap {
		cont := p.perAddress[el.stateTx.Tx.Transaction.From]
		cont.item = el
		p.perAddress[el.stateTx.Tx.Transaction.From] = cont
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

type aaPoolAddressData struct {
	item *aaPoolTimeHeapItem
	pool *aaPoolNonceHeap
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
