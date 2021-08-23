package jsonrpc

import (
	"container/heap"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/0xPolygon/polygon-sdk/blockchain"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
)

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

	// websocket connection
	ws wsConn
}

func (f *Filter) getFilterUpdates() (string, error) {
	if f.isBlockFilter() {
		// block filter
		headers, newHead := f.block.getUpdates()
		f.block = newHead

		updates := []string{}
		for _, header := range headers {
			updates = append(updates, header.Hash.String())
		}
		return fmt.Sprintf("[\"%s\"]", strings.Join(updates, "\",\"")), nil
	}
	// log filter
	res, err := json.Marshal(f.logs)
	if err != nil {
		return "", err
	}
	f.logs = []*Log{}
	return string(res), nil
}

func (f *Filter) isWS() bool {
	return f.ws != nil
}

var ethSubscriptionTemplate = `{
	"jsonrpc": "2.0",
	"method": "eth_subscription",
	"params": {
		"subscription":"%s",
		"result": %s
	}
}`

func (f *Filter) sendMessage(msg string) error {
	res := fmt.Sprintf(ethSubscriptionTemplate, f.id, msg)
	if err := f.ws.WriteMessage(websocket.TextMessage, []byte(res)); err != nil {
		return err
	}
	return nil
}

func (f *Filter) flush() error {
	if f.isBlockFilter() {
		// send each block independently
		updates, newHead := f.block.getUpdates()
		f.block = newHead

		for _, block := range updates {
			raw, err := json.Marshal(block)
			if err != nil {
				return err
			}
			if err := f.sendMessage(string(raw)); err != nil {
				return err
			}
		}
	} else {
		// log filter
		for _, log := range f.logs {
			res, err := json.Marshal(log)
			if err != nil {
				return err
			}
			if err := f.sendMessage(string(res)); err != nil {
				return err
			}
		}
		f.logs = []*Log{}
	}
	return nil
}

func (f *Filter) isLogFilter() bool {
	return f.logFilter != nil
}

func (f *Filter) isBlockFilter() bool {
	return f.block != nil
}

var defaultTimeout = 1 * time.Minute

type FilterManager struct {
	logger hclog.Logger

	store   blockchainInterface
	closeCh chan struct{}

	subscription blockchain.Subscription

	filters map[string]*Filter
	lock    sync.Mutex

	updateCh chan struct{}
	timer    timeHeapImpl
	timeout  time.Duration

	blockStream *blockStream
}

func NewFilterManager(logger hclog.Logger, store blockchainInterface) *FilterManager {
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
	m.blockStream.push(header)

	// start the head watcher
	m.subscription = store.SubscribeEvents()

	return m
}

func (f *FilterManager) Run() {

	// watch for new events in the blockchain
	watchCh := make(chan *blockchain.Event)
	go func() {
		for {
			evnt := f.subscription.GetEvent()
			if evnt == nil {
				return
			}
			watchCh <- evnt
		}
	}()

	var timeoutCh <-chan time.Time
	for {
		// check for the next filter to be removed
		filter := f.nextTimeoutFilter()
		if filter == nil {
			timeoutCh = nil
		} else {
			timeoutCh = time.After(time.Until(filter.timestamp))
		}

		select {
		case evnt := <-watchCh:
			// new blockchain event
			if err := f.dispatchEvent(evnt); err != nil {
				f.logger.Error("failed to dispatch event", "err", err)
			}

		case <-timeoutCh:
			// timeout for filter
			if !f.Uninstall(filter.id) {
				f.logger.Error("failed to uninstall filter", "id", filter.id)
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

func (f *FilterManager) dispatchEvent(evnt *blockchain.Event) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	// first include all the new headers in the blockstream for the block filters
	for _, header := range evnt.NewChain {
		f.blockStream.push(header)
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
								Address:     log.Address,
								Topics:      log.Topics,
								Data:        argBytes(log.Data),
								BlockNumber: argUint64(h.Number),
								BlockHash:   h.Hash,
								TxHash:      receipt.TxHash,
								TxIndex:     argUint64(indx),
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

	// flush all the websocket values
	for _, f := range f.filters {
		if f.isWS() {
			f.flush()
		}
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
	if item.isWS() {
		// we cannot get updates from a ws filter with getFilterChanges
		return "", errFilterDoesNotExists
	}

	res, err := item.getFilterUpdates()
	if err != nil {
		return "", err
	}
	return res, nil
}

func (f *FilterManager) Uninstall(id string) bool {
	f.lock.Lock()

	item, ok := f.filters[id]
	if !ok {
		return false
	}

	delete(f.filters, id)
	heap.Remove(&f.timer, item.index)

	f.lock.Unlock()
	return true
}

func (f *FilterManager) NewBlockFilter(ws wsConn) string {
	return f.addFilter(nil, ws)
}

func (f *FilterManager) NewLogFilter(logFilter *LogFilter, ws wsConn) string {
	return f.addFilter(logFilter, ws)
}

func (f *FilterManager) addFilter(logFilter *LogFilter, ws wsConn) string {
	f.lock.Lock()

	filter := &Filter{
		id: uuid.New().String(),
		ws: ws,
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

func (b *blockStream) push(header *types.Header) {
	b.lock.Lock()
	newHead := &headElem{
		header: header.Copy(),
	}
	if b.head != nil {
		b.head.next = newHead
	}
	b.head = newHead
	b.lock.Unlock()
}

type headElem struct {
	header *types.Header
	next   *headElem
}

func (h *headElem) getUpdates() ([]*types.Header, *headElem) {
	res := []*types.Header{}

	cur := h
	for {
		if cur.next == nil {
			break
		}
		cur = cur.next
		res = append(res, cur.header)
	}
	return res, cur
}
