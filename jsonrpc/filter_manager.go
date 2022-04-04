package jsonrpc

import (
	"container/heap"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
)

var ErrFilterDoesNotExists = errors.New("filter does not exists")

// filter is an interface that BlockFilter and LogFilter will implement
type filter interface {
	isWS() bool
	getFilterBase() *filterBase
	getUpdates() (string, error)
	sendUpdates() error
}

// filterBase is a struct that has common fields of BlockFilter and LogFilter
type filterBase struct {
	// UUID, a key of filter for client
	id string

	// index in the timeouts heap, -1 for non-existing index
	index int

	// timestamp to expire
	expiredAt time.Time

	// websocket connection
	ws wsConn
}

// newFilterBase initializes filterBase with unique ID
func newFilterBase(ws wsConn) filterBase {
	return filterBase{
		id:    uuid.New().String(),
		ws:    ws,
		index: -1,
	}
}

// getFilterBase returns the own reference so that child struct can return base
func (f *filterBase) getFilterBase() *filterBase {
	return f
}

// isWS returns the flag indicating this filter has websocket connection
func (f *filterBase) isWS() bool {
	return f.ws != nil
}

// writeMessageToWs sends given message to websocket stream
func (f *filterBase) writeMessageToWs(msg string) error {
	res := fmt.Sprintf(ethSubscriptionTemplate, f.id, msg)
	if err := f.ws.WriteMessage(websocket.TextMessage, []byte(res)); err != nil {
		return err
	}

	return nil
}

type BlockFilter struct {
	filterBase
	block *headElem
}

func (f *BlockFilter) takeBlockUpdates() []*types.Header {
	updates, newHead := f.block.getUpdates()
	f.block = newHead

	return updates
}

func (f *BlockFilter) getUpdates() (string, error) {
	headers := f.takeBlockUpdates()

	updates := []string{}
	for _, header := range headers {
		updates = append(updates, header.Hash.String())
	}

	return fmt.Sprintf("[\"%s\"]", strings.Join(updates, "\",\"")), nil
}

func (f *BlockFilter) sendUpdates() error {
	updates := f.takeBlockUpdates()

	for _, block := range updates {
		raw, err := json.Marshal(block)
		if err != nil {
			return err
		}

		if err := f.writeMessageToWs(string(raw)); err != nil {
			return err
		}
	}

	return nil
}

type LogFilter struct {
	filterBase
	query *LogQuery
	logs  []*Log
}

func (f *LogFilter) appendLog(log *Log) {
	// TODO: lock
	f.logs = append(f.logs, log)
}

func (f *LogFilter) getUpdates() (string, error) {
	res, err := json.Marshal(f.logs)
	if err != nil {
		return "", err
	}

	f.logs = f.logs[:0]

	return string(res), nil
}

func (f *LogFilter) sendUpdates() error {
	for _, log := range f.logs {
		res, err := json.Marshal(log)
		if err != nil {
			return err
		}

		if err := f.writeMessageToWs(string(res)); err != nil {
			return err
		}
	}

	f.logs = f.logs[:0]

	return nil
}

var ethSubscriptionTemplate = `{
	"jsonrpc": "2.0",
	"method": "eth_subscription",
	"params": {
		"subscription":"%s",
		"result": %s
	}
}`

var defaultTimeout = 1 * time.Minute

// filterManagerStore provides methods required by FilterManager
type filterManagerStore interface {
	// Header returns the current header of the chain (genesis if empty)
	Header() *types.Header

	// SubscribeEvents subscribes for chain head events
	SubscribeEvents() blockchain.Subscription

	// GetReceiptsByHash returns the receipts for a block hash
	GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error)
}

type FilterManager struct {
	logger hclog.Logger

	timeout time.Duration

	store        filterManagerStore
	subscription blockchain.Subscription
	blockStream  *blockStream

	lock     sync.RWMutex
	filters  map[string]filter
	timeouts timeHeapImpl

	updateCh chan struct{}
	closeCh  chan struct{}
}

func NewFilterManager(logger hclog.Logger, store filterManagerStore) *FilterManager {
	m := &FilterManager{
		logger:      logger.Named("filter"),
		timeout:     defaultTimeout,
		store:       store,
		blockStream: &blockStream{},

		// filters
		lock:     sync.RWMutex{},
		filters:  make(map[string]filter),
		timeouts: timeHeapImpl{},

		updateCh: make(chan struct{}),
		closeCh:  make(chan struct{}),
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
		filterBase := f.nextTimeoutFilter()

		//	uninstall filters only
		if filterBase != nil {
			timeoutCh = time.After(time.Until(filterBase.expiredAt))
		}

		select {
		case evnt := <-watchCh:
			// new blockchain event
			if err := f.dispatchEvent(evnt); err != nil {
				f.logger.Error("failed to dispatch event", "err", err)
			}

		case <-timeoutCh:
			// timeout for filter
			if !f.Uninstall(filterBase.id) {
				f.logger.Error("failed to uninstall filter", "id", filterBase.id)
			}

		case <-f.updateCh:
			// there is a new filter, reset the loop to start the timeout timer

		case <-f.closeCh:
			// stop the filter manager
			return
		}
	}
}

func (f *FilterManager) nextTimeoutFilter() *filterBase {
	f.lock.RLock()
	defer f.lock.RUnlock()

	if len(f.timeouts) == 0 {
		return nil
	}

	// peek the first item
	base := f.timeouts[0]

	return base
}

func (f *FilterManager) dispatchEvent(evnt *blockchain.Event) error {
	if err := f.processEvent(evnt); err != nil {
		return err
	}

	// send data to WebSocket streams
	if err := f.flushWsFilters(); err != nil {
		return err
	}

	return nil
}

func (f *FilterManager) processEvent(evnt *blockchain.Event) error {
	f.lock.RLock()
	defer f.lock.RUnlock()

	// first include all the new headers in the blockstream for BlockFilter
	for _, header := range evnt.NewChain {
		f.blockStream.push(header)
	}

	// process old chain to include old logs marked removed for LogFilter
	for _, header := range evnt.OldChain {
		if processErr := f.appendLogsToFilters(header, true); processErr != nil {
			f.logger.Error(fmt.Sprintf("Unable to process block, %v", processErr))
		}
	}

	// process new chain to include new logs for LogFilter
	for _, header := range evnt.NewChain {
		if processErr := f.appendLogsToFilters(header, false); processErr != nil {
			f.logger.Error(fmt.Sprintf("Unable to process block, %v", processErr))
		}
	}

	return nil
}

func (f *FilterManager) appendLogsToFilters(header *types.Header, removed bool) error {
	receipts, err := f.store.GetReceiptsByHash(header.Hash)
	if err != nil {
		return err
	}

	for indx, receipt := range receipts {
		// check the logs with the filters
		for _, log := range receipt.Logs {
			nn := &Log{
				Address:     log.Address,
				Topics:      log.Topics,
				Data:        argBytes(log.Data),
				BlockNumber: argUint64(header.Number),
				BlockHash:   header.Hash,
				TxHash:      receipt.TxHash,
				TxIndex:     argUint64(indx),
				Removed:     removed,
			}

			for _, f := range f.filters {
				if logFilter, ok := f.(*LogFilter); ok {
					logFilter.appendLog(nn)
				}
			}
		}
	}

	return nil
}

func (f *FilterManager) flushWsFilters() error {
	closedFilterIDs := make([]string, 0)

	f.lock.RLock()

	for id, filter := range f.filters {
		if !filter.isWS() {
			continue
		}

		if flushErr := filter.sendUpdates(); flushErr != nil {
			if errors.Is(flushErr, websocket.ErrCloseSent) {
				closedFilterIDs = append(closedFilterIDs, id)

				f.logger.Warn(fmt.Sprintf("Subscription %s has been closed", id))

				continue
			}

			f.logger.Error(fmt.Sprintf("Unable to process flush, %v", flushErr))
		}
	}

	f.lock.RUnlock()

	if len(closedFilterIDs) > 0 {
		f.lock.Lock()

		for _, id := range closedFilterIDs {
			f.removeFilterByID(id)
		}

		f.lock.Unlock()
		f.emitSignalToUpdateCh()
		f.logger.Info(fmt.Sprintf("Removed %d filters due to closed connections", len(closedFilterIDs)))
	}

	return nil
}

func (f *FilterManager) Exists(id string) bool {
	f.lock.RLock()
	defer f.lock.RUnlock()

	_, ok := f.filters[id]

	return ok
}

func (f *FilterManager) GetFilterChanges(id string) (string, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()

	filter, ok := f.filters[id]

	// we cannot get updates from a ws filter with getFilterChanges
	if !ok || filter.isWS() {
		return "", ErrFilterDoesNotExists
	}

	res, err := filter.getUpdates()
	if err != nil {
		return "", err
	}

	return res, nil
}

func (f *FilterManager) Uninstall(id string) bool {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.removeFilterByID(id)
}

// removeFilterByID removes a filter with given ID, unsafe against race condition
func (f *FilterManager) removeFilterByID(id string) bool {
	filter, ok := f.filters[id]
	if !ok {
		return false
	}

	delete(f.filters, id)

	if removed := f.timeouts.removeFilter(filter.getFilterBase()); removed {
		f.emitSignalToUpdateCh()
	}

	return true
}

func (f *FilterManager) NewBlockFilter(ws wsConn) string {
	filter := &BlockFilter{
		filterBase: newFilterBase(ws),
		block:      f.blockStream.Head(),
	}

	return f.addFilter(filter)
}

func (f *FilterManager) NewLogFilter(logQuery *LogQuery, ws wsConn) string {
	filter := &LogFilter{
		filterBase: newFilterBase(ws),
		query:      logQuery,
	}

	return f.addFilter(filter)
}

func (f *FilterManager) addFilter(filter filter) string {
	f.lock.Lock()
	defer f.lock.Unlock()

	base := filter.getFilterBase()

	f.filters[base.id] = filter

	// Set timeout if filter is not WS
	if !filter.isWS() {
		f.timeouts.addFilter(base, f.timeout)
		f.emitSignalToUpdateCh()
	}

	return base.id
}

func (f *FilterManager) emitSignalToUpdateCh() {
	select {
	// notify worker of new filter with timeout
	case f.updateCh <- struct{}{}:
	default:
	}
}

func (f *FilterManager) Close() {
	close(f.closeCh)
}

type timeHeapImpl []*filterBase

func (t *timeHeapImpl) addFilter(filter *filterBase, timeout time.Duration) {
	filter.expiredAt = time.Now().Add(timeout)
	heap.Push(t, filter)
}

func (t *timeHeapImpl) removeFilter(filter *filterBase) bool {
	if filter.index == -1 {
		return false
	}

	heap.Remove(t, filter.index)

	return true
}

func (t timeHeapImpl) Len() int { return len(t) }

func (t timeHeapImpl) Less(i, j int) bool {
	return t[i].expiredAt.Before(t[j].expiredAt)
}

func (t timeHeapImpl) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
	t[i].index = i
	t[j].index = j
}

func (t *timeHeapImpl) Push(x interface{}) {
	n := len(*t)
	item := x.(*filterBase) //nolint: forcetypeassert
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
	defer b.lock.Unlock()

	return b.head
}

func (b *blockStream) push(header *types.Header) {
	b.lock.Lock()
	defer b.lock.Unlock()

	newHead := &headElem{
		header: header.Copy(),
	}

	if b.head != nil {
		b.head.next = newHead
	}

	b.head = newHead
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
