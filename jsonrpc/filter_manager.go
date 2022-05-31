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

var (
	ErrFilterDoesNotExists              = errors.New("filter does not exists")
	ErrWSFilterDoesNotSupportGetChanges = errors.New("web socket Filter doesn't support to return a batch of the changes")
	ErrCastingFilterToLogFilter         = errors.New("casting filter object to logFilter error")
	ErrBlockNotFound                    = errors.New("block not found")
	ErrIncorrectBlockRange              = errors.New("incorrect range")
	ErrPendingBlockNumber               = errors.New("pending block number is not supported")
)

// defaultTimeout is the timeout to remove the filters that don't have a web socket stream
var defaultTimeout = 1 * time.Minute

const (
	// The index in heap which is indicating the element is not in the heap
	NoIndexInHeap = -1
)

// filter is an interface that BlockFilter and LogFilter implement
type filter interface {
	// isWS returns the flag indicating the filter has web socket stream
	isWS() bool

	// getFilterBase returns filterBase that has common fields
	getFilterBase() *filterBase

	// getUpdates returns stored data in string
	getUpdates() (string, error)

	// sendUpdates write stored data to web socket stream
	sendUpdates() error
}

// filterBase is a struct for common fields between BlockFilter and LogFilter
type filterBase struct {
	// UUID, a key of filter for client
	id string

	// index in the timeouts heap, -1 for non-existing index
	heapIndex int

	// timestamp to be expired
	expiredAt time.Time

	// websocket connection
	ws wsConn
}

// newFilterBase initializes filterBase with unique ID
func newFilterBase(ws wsConn) filterBase {
	return filterBase{
		id:        uuid.New().String(),
		ws:        ws,
		heapIndex: NoIndexInHeap,
	}
}

// getFilterBase returns its own reference so that child struct can return base
func (f *filterBase) getFilterBase() *filterBase {
	return f
}

// isWS returns the flag indicating this filter has websocket connection
func (f *filterBase) isWS() bool {
	return f.ws != nil
}

const ethSubscriptionTemplate = `{
	"jsonrpc": "2.0",
	"method": "eth_subscription",
	"params": {
		"subscription":"%s",
		"result": %s
	}
}`

// writeMessageToWs sends given message to websocket stream
func (f *filterBase) writeMessageToWs(msg string) error {
	res := fmt.Sprintf(ethSubscriptionTemplate, f.id, msg)
	if err := f.ws.WriteMessage(websocket.TextMessage, []byte(res)); err != nil {
		return err
	}

	return nil
}

// blockFilter is a filter to store the updates of block
type blockFilter struct {
	filterBase
	sync.Mutex
	block *headElem
}

// takeBlockUpdates advances blocks from head to latest and returns header array
func (f *blockFilter) takeBlockUpdates() []*types.Header {
	updates, newHead := f.block.getUpdates()

	f.Lock()
	f.block = newHead
	f.Unlock()

	return updates
}

// getUpdates returns updates of blocks in string
func (f *blockFilter) getUpdates() (string, error) {
	headers := f.takeBlockUpdates()

	updates := []string{}
	for _, header := range headers {
		updates = append(updates, header.Hash.String())
	}

	return fmt.Sprintf("[\"%s\"]", strings.Join(updates, "\",\"")), nil
}

// sendUpdates writes the updates of blocks to web socket stream
func (f *blockFilter) sendUpdates() error {
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

// logFilter is a filter to store logs that meet the conditions in query
type logFilter struct {
	filterBase
	sync.Mutex
	query *LogQuery
	logs  []*Log
}

// appendLog appends new log to logs
func (f *logFilter) appendLog(log *Log) {
	f.Lock()
	defer f.Unlock()

	f.logs = append(f.logs, log)
}

// takeLogUpdates returns all saved logs in filter and set new log slice
func (f *logFilter) takeLogUpdates() []*Log {
	f.Lock()
	defer f.Unlock()

	logs := f.logs
	f.logs = []*Log{} // create brand new slice so that prevent new logs from being added to current logs

	return logs
}

// getUpdates returns stored logs in string
func (f *logFilter) getUpdates() (string, error) {
	logs := f.takeLogUpdates()

	res, err := json.Marshal(logs)
	if err != nil {
		return "", err
	}

	return string(res), nil
}

// sendUpdates writes stored logs to web socket stream
func (f *logFilter) sendUpdates() error {
	logs := f.takeLogUpdates()

	for _, log := range logs {
		res, err := json.Marshal(log)
		if err != nil {
			return err
		}

		if err := f.writeMessageToWs(string(res)); err != nil {
			return err
		}
	}

	return nil
}

// filterManagerStore provides methods required by FilterManager
type filterManagerStore interface {
	// Header returns the current header of the chain (genesis if empty)
	Header() *types.Header

	// SubscribeEvents subscribes for chain head events
	SubscribeEvents() blockchain.Subscription

	// GetReceiptsByHash returns the receipts for a block hash
	GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error)

	// GetBlockByHash returns the block using the block hash
	GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool)

	// GetBlockByNumber returns a block using the provided number
	GetBlockByNumber(num uint64, full bool) (*types.Block, bool)
}

// FilterManager manages all running filters
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
		lock:        sync.RWMutex{},
		filters:     make(map[string]filter),
		timeouts:    timeHeapImpl{},
		updateCh:    make(chan struct{}),
		closeCh:     make(chan struct{}),
	}

	// start blockstream with the current header
	header := store.Header()
	m.blockStream.push(header)

	// start the head watcher
	m.subscription = store.SubscribeEvents()

	return m
}

// Run starts worker process to handle events
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

		// set timer to remove filter
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
			// if filter still exists
			if filterBase != nil && !f.Uninstall(filterBase.id) {
				f.logger.Error("failed to uninstall filter", "id", filterBase.id)
			}

		case <-f.updateCh:
			// filters change, reset the loop to start the timeout timer

		case <-f.closeCh:
			// stop the filter manager
			return
		}
	}
}

// Close closed closeCh so that terminate worker
func (f *FilterManager) Close() {
	close(f.closeCh)
}

// NewBlockFilter adds new BlockFilter
func (f *FilterManager) NewBlockFilter(ws wsConn) string {
	filter := &blockFilter{
		filterBase: newFilterBase(ws),
		block:      f.blockStream.Head(),
	}

	return f.addFilter(filter)
}

// NewLogFilter adds new LogFilter
func (f *FilterManager) NewLogFilter(logQuery *LogQuery, ws wsConn) string {
	filter := &logFilter{
		filterBase: newFilterBase(ws),
		query:      logQuery,
	}

	return f.addFilter(filter)
}

// Exists checks the filter with given ID exists
func (f *FilterManager) Exists(id string) bool {
	f.lock.RLock()
	defer f.lock.RUnlock()

	_, ok := f.filters[id]

	return ok
}

func (f *FilterManager) getLogsFromBlock(query *LogQuery, block *types.Block) ([]*Log, error) {
	receipts, err := f.store.GetReceiptsByHash(block.Header.Hash)
	if err != nil {
		return nil, err
	}

	logs := make([]*Log, 0)

	for idx, receipt := range receipts {
		for logIdx, log := range receipt.Logs {
			if query.Match(log) {
				logs = append(logs, &Log{
					Address:     log.Address,
					Topics:      log.Topics,
					Data:        argBytes(log.Data),
					BlockNumber: argUint64(block.Header.Number),
					BlockHash:   block.Header.Hash,
					TxHash:      block.Transactions[idx].Hash,
					TxIndex:     argUint64(idx),
					LogIndex:    argUint64(logIdx),
				})
			}
		}
	}

	return logs, nil
}

func (f *FilterManager) getLogsFromBlocks(query *LogQuery) ([]*Log, error) {
	latestBlockNumber := f.store.Header().Number

	resolveNum := func(num BlockNumber) (uint64, error) {
		switch num {
		case PendingBlockNumber:
			return 0, ErrPendingBlockNumber
		case EarliestBlockNumber:
			num = 0
		case LatestBlockNumber:
			return latestBlockNumber, nil
		}

		return uint64(num), nil
	}

	from, err := resolveNum(query.fromBlock)
	if err != nil {
		return nil, err
	}

	to, err := resolveNum(query.toBlock)
	if err != nil {
		return nil, err
	}

	if to < from {
		return nil, ErrIncorrectBlockRange
	}

	// If from equals genesis block
	// skip it
	if from == 0 {
		from = 1
	}

	logs := make([]*Log, 0)

	for i := from; i <= to; i++ {
		block, ok := f.store.GetBlockByNumber(i, true)
		if !ok {
			break
		}

		if len(block.Transactions) == 0 {
			// do not check logs if no txs
			continue
		}

		blockLogs, err := f.getLogsFromBlock(query, block)
		if err != nil {
			return nil, err
		}

		logs = append(logs, blockLogs...)
	}

	return logs, nil
}

// GetLogsForQuery return array of logs for given query
func (f *FilterManager) GetLogsForQuery(query *LogQuery) ([]*Log, error) {
	if query.BlockHash != nil {
		//	BlockHash is set -> fetch logs from this block only
		block, ok := f.store.GetBlockByHash(*query.BlockHash, true)
		if !ok {
			return nil, ErrBlockNotFound
		}

		if len(block.Transactions) == 0 {
			// no txs in block, return empty response
			return []*Log{}, nil
		}

		return f.getLogsFromBlock(query, block)
	}

	//	gets logs from a range of blocks
	return f.getLogsFromBlocks(query)
}

//GetLogFilterFromID return log filter for given filterID
func (f *FilterManager) GetLogFilterFromID(filterID string) (*logFilter, error) {
	f.lock.RLock()

	filter, ok := f.filters[filterID]
	f.lock.RUnlock()

	if !ok {
		return nil, ErrFilterDoesNotExists
	}

	logFilter, ok := filter.(*logFilter)
	if !ok {
		return nil, ErrCastingFilterToLogFilter
	}

	return logFilter, nil
}

// GetFilterChanges returns the updates of the filter with given ID in string
func (f *FilterManager) GetFilterChanges(id string) (string, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()

	filter, ok := f.filters[id]

	if !ok {
		return "", ErrFilterDoesNotExists
	}

	// we cannot get updates from a ws filter with getFilterChanges
	if filter.isWS() {
		return "", ErrWSFilterDoesNotSupportGetChanges
	}

	res, err := filter.getUpdates()
	if err != nil {
		return "", err
	}

	return res, nil
}

// Uninstall removes the filter with given ID from list
func (f *FilterManager) Uninstall(id string) bool {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.removeFilterByID(id)
}

// removeFilterByID removes the filter with given ID, unsafe against race condition
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

// addFilter is an internal method to add given filter to list and heap
func (f *FilterManager) addFilter(filter filter) string {
	f.lock.Lock()
	defer f.lock.Unlock()

	base := filter.getFilterBase()

	f.filters[base.id] = filter

	// Set timeout and add to heap if filter doesn't have web socket connection
	if !filter.isWS() {
		base.expiredAt = time.Now().Add(f.timeout)
		f.timeouts.addFilter(base)
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

// nextTimeoutFilter returns the filter that will be expired next
// nextTimeoutFilter returns the only filter with timeout
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

// dispatchEvent is a event handler for new block event
func (f *FilterManager) dispatchEvent(evnt *blockchain.Event) error {
	// store new event in each filters
	if err := f.processEvent(evnt); err != nil {
		return err
	}

	// send data to web socket stream
	if err := f.flushWsFilters(); err != nil {
		return err
	}

	return nil
}

// processEvent makes each filter append the new data that interests them
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

// appendLogsToFilters makes each LogFilters append logs in the header
func (f *FilterManager) appendLogsToFilters(header *types.Header, removed bool) error {
	receipts, err := f.store.GetReceiptsByHash(header.Hash)
	if err != nil {
		return err
	}

	// Get logFilters from filters
	logFilters := f.getLogFilters()
	if len(logFilters) == 0 {
		return nil
	}

	block, ok := f.store.GetBlockByHash(header.Hash, true)
	if !ok {
		f.logger.Error("could not find block in store", "hash", header.Hash.String())

		return nil
	}

	for indx, receipt := range receipts {
		if receipt.TxHash == types.ZeroHash {
			// Extract tx Hash
			receipt.TxHash = block.Transactions[indx].Hash
		}
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

			for _, f := range logFilters {
				if f.query.Match(log) {
					f.appendLog(nn)
				}
			}
		}
	}

	return nil
}

// flushWsFilters make each filters with web socket connection write the updates to web socket stream
// flushWsFilters also removes the filters if flushWsFilters notices the connection is closed
func (f *FilterManager) flushWsFilters() error {
	closedFilterIDs := make([]string, 0)

	f.lock.RLock()

	for id, filter := range f.filters {
		if !filter.isWS() {
			continue
		}

		if flushErr := filter.sendUpdates(); flushErr != nil {
			// mark as closed if the connection is closed
			if errors.Is(flushErr, websocket.ErrCloseSent) {
				closedFilterIDs = append(closedFilterIDs, id)

				f.logger.Warn(fmt.Sprintf("Subscription %s has been closed", id))

				continue
			}

			f.logger.Error(fmt.Sprintf("Unable to process flush, %v", flushErr))
		}
	}

	f.lock.RUnlock()

	// remove filters with closed web socket connections from FilterManager
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

// getLogFilters returns logFilters
func (f *FilterManager) getLogFilters() []*logFilter {
	f.lock.RLock()
	defer f.lock.RUnlock()

	logFilters := []*logFilter{}

	for _, f := range f.filters {
		if logFilter, ok := f.(*logFilter); ok {
			logFilters = append(logFilters, logFilter)
		}
	}

	return logFilters
}

type timeHeapImpl []*filterBase

func (t *timeHeapImpl) addFilter(filter *filterBase) {
	heap.Push(t, filter)
}

func (t *timeHeapImpl) removeFilter(filter *filterBase) bool {
	if filter.heapIndex == NoIndexInHeap {
		return false
	}

	heap.Remove(t, filter.heapIndex)

	return true
}

func (t timeHeapImpl) Len() int { return len(t) }

func (t timeHeapImpl) Less(i, j int) bool {
	return t[i].expiredAt.Before(t[j].expiredAt)
}

func (t timeHeapImpl) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
	t[i].heapIndex = i
	t[j].heapIndex = j
}

func (t *timeHeapImpl) Push(x interface{}) {
	n := len(*t)
	item := x.(*filterBase) //nolint: forcetypeassert
	item.heapIndex = n
	*t = append(*t, item)
}

func (t *timeHeapImpl) Pop() interface{} {
	old := *t
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.heapIndex = -1
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
