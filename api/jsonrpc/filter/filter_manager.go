package filter

import (
	"container/heap"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/types"
	"github.com/google/uuid"
	"github.com/hashicorp/go-hclog"
)

type Log struct {
	types.Log

	BlockNumber uint64     `json:"blockNumber"`
	TxHash      types.Hash `json:"transactionHash"`
	TxIndex     uint       `json:"transactionIndex"`
	BlockHash   types.Hash `json:"blockHash"`
	LogIndex    uint       `json:"logIndex"`
	Removed     bool       `json:"removed"`
}

type Filter struct {
	id string

	// block filter
	block *headElem

	// log cache
	logs []*Log

	// log filter
	logFilter *LogFilter

	// index of the filter in the timer array
	index int

	// next time to timeout
	timestamp time.Time
}

func (f *Filter) isLogFilter() bool {
	return f.logFilter != nil
}

func (f *Filter) isBlockFilter() bool {
	return f.block != nil
}

func (f *Filter) match() bool {
	return false
}

type subscription interface {
	Watch() chan blockchain.Event
	Close()
}

// blockchain is the interface with the blockchain required
// by the filter manager
type store interface {
	// Header returns the current header of the chain (genesis if empty)
	Header() *types.Header

	// GetReceiptsByHash returns the receipts for a hash
	GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error)

	// Subscribe subscribes for chain head events
	Subscribe() subscription
}

var defaultTimeout = 1 * time.Minute

type FilterManager struct {
	logger hclog.Logger

	store   store
	closeCh chan struct{}

	watcher chan blockchain.Event

	filters map[string]*Filter
	lock    sync.Mutex

	updateCh chan struct{}
	timer    timeHeapImpl
	timeout  time.Duration

	blockStream *blockStream
}

func NewFilterManager(logger hclog.Logger, store store) *FilterManager {
	m := &FilterManager{
		logger:      logger.Named("filter"),
		store:       store,
		closeCh:     make(chan struct{}),
		filters:     map[string]*Filter{},
		updateCh:    make(chan struct{}),
		timer:       timeHeapImpl{},
		blockStream: &blockStream{},
		timeout:     defaultTimeout,
	}

	// start blockstream with the current header
	header := store.Header()
	m.blockStream.push(header.Hash)

	// start the head watcher
	m.watcher = store.Subscribe().Watch()

	return m
}

func (f *FilterManager) Run() {
	// watch for new events in the blockchain

	var timeoutCh <-chan time.Time
	for {
		// check for the next filter to be removed
		filter := f.nextTimeoutFilter()
		if filter == nil {
			timeoutCh = nil
		} else {
			timeoutCh = time.After(filter.timestamp.Sub(time.Now()))
		}

		select {
		case evnt := <-f.watcher:
			// new blockchain event
			if err := f.dispatchEvent(evnt); err != nil {
				f.logger.Error("failed to dispatch event", "err", err)
			}

		case <-timeoutCh:
			// timeout for filter
			if err := f.Uninstall(filter.id); err != nil {
				f.logger.Error("failed to uninstall filter", "err", err)
			}

		case <-f.updateCh:
			// there is a new filter, reset the loop to start the timeout timer

		case <-f.closeCh:
			// stop the filter manager
			return
		}
	}
}

func (f *FilterManager) nextTimeoutFilter() *Filter {
	f.lock.Lock()
	if len(f.filters) == 0 {
		f.lock.Unlock()
		return nil
	}

	// pop the first item
	item := f.timer[0]
	f.lock.Unlock()
	return item
}

func (f *FilterManager) dispatchEvent(evnt blockchain.Event) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	// first include all the new headers in the blockstream for the block filters
	for _, header := range evnt.NewChain {
		f.blockStream.push(header.Hash)
	}

	processBlock := func(h *types.Header, removed bool) error {
		// get the logs from the transaction
		receipts, err := f.store.GetReceiptsByHash(h.Hash)
		if err != nil {
			return err
		}

		for indx, receipt := range receipts {
			// check the logs with the filters
			for _, log := range receipt.Logs {
				for _, f := range f.filters {
					if f.isLogFilter() {
						if f.logFilter.Match(log) {
							nn := &Log{
								Log:         *log,
								BlockNumber: h.Number,
								BlockHash:   h.Hash,
								TxHash:      receipt.TxHash,
								TxIndex:     uint(indx),
								Removed:     removed,
							}
							f.logs = append(f.logs, nn)
						}
					}
				}
			}
		}
		return nil
	}

	// process old chain
	for _, i := range evnt.OldChain {
		processBlock(i, true)
	}
	// process new chain
	for _, i := range evnt.NewChain {
		processBlock(i, false)
	}

	return nil
}

func (f *FilterManager) Exists(id string) bool {
	f.lock.Lock()
	_, ok := f.filters[id]
	f.lock.Unlock()
	return ok
}

var errFilterDoesNotExists = fmt.Errorf("filter does not exists")

func (f *FilterManager) GetFilterChanges(id string) (string, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	item, ok := f.filters[id]
	if !ok {
		return "", errFilterDoesNotExists
	}

	if !item.isBlockFilter() {
		// log filter
		res, err := json.Marshal(item.logs)
		if err != nil {
			return "", err
		}
		return string(res), nil
	}

	updates, newHead := item.block.getUpdates()
	item.block = newHead

	res := fmt.Sprintf("[\"%s\"]", strings.Join(updates, "\",\""))

	return res, nil
}

func (f *FilterManager) Uninstall(id string) error {
	f.lock.Lock()

	item, ok := f.filters[id]
	if !ok {
		return errFilterDoesNotExists
	}

	delete(f.filters, id)
	heap.Remove(&f.timer, item.index)

	f.lock.Unlock()
	return nil
}

func (f *FilterManager) NewBlockFilter() string {
	return f.addFilter(nil)
}

func (f *FilterManager) NewLogFilter(logFilter *LogFilter) string {
	return f.addFilter(logFilter)
}

func (f *FilterManager) addFilter(logFilter *LogFilter) string {
	f.lock.Lock()

	filter := &Filter{
		id: uuid.New().String(),
	}

	if logFilter == nil {
		// block filter
		// take the reference from the stream
		filter.block = f.blockStream.Head()
	} else {
		// log filter
		filter.logFilter = logFilter
	}

	f.filters[filter.id] = filter
	filter.timestamp = time.Now().Add(f.timeout)
	heap.Push(&f.timer, filter)

	f.lock.Unlock()

	select {
	case f.updateCh <- struct{}{}:
	default:
	}

	return filter.id
}

func (f *FilterManager) Close() {
	close(f.closeCh)
}

type timeHeapImpl []*Filter

func (t timeHeapImpl) Len() int { return len(t) }

func (t timeHeapImpl) Less(i, j int) bool {
	return t[i].timestamp.Before(t[j].timestamp)
}

func (t timeHeapImpl) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
	t[i].index = i
	t[j].index = j
}

func (t *timeHeapImpl) Push(x interface{}) {
	n := len(*t)
	item := x.(*Filter)
	item.index = n
	*t = append(*t, item)
}

func (t *timeHeapImpl) Pop() interface{} {
	old := *t
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*t = old[0 : n-1]
	return item
}

// blockStream is used to keep the stream of new block hashes and allow subscriptions
// of the stream at any point
type blockStream struct {
	lock sync.Mutex
	head *headElem
}

func (b *blockStream) Head() *headElem {
	b.lock.Lock()
	head := b.head
	b.lock.Unlock()
	return head
}

func (b *blockStream) push(newBlock types.Hash) {
	b.lock.Lock()
	newHead := &headElem{
		hash: newBlock.String(),
	}
	if b.head != nil {
		b.head.next = newHead
	}
	b.head = newHead
	b.lock.Unlock()
}

type headElem struct {
	hash string
	next *headElem
}

func (h *headElem) getUpdates() ([]string, *headElem) {
	res := []string{}

	cur := h
	for {
		if cur.next == nil {
			break
		}
		cur = cur.next
		res = append(res, cur.hash)
	}
	return res, cur
}
