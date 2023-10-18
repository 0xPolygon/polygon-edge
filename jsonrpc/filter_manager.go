package jsonrpc

import (
	"container/heap"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
)

var (
	ErrFilterNotFound                   = errors.New("filter not found")
	ErrWSFilterDoesNotSupportGetChanges = errors.New("web socket Filter doesn't support to return a batch of the changes")
	ErrCastingFilterToLogFilter         = errors.New("casting filter object to logFilter error")
	ErrBlockNotFound                    = errors.New("block not found")
	ErrIncorrectBlockRange              = errors.New("incorrect range")
	ErrBlockRangeTooHigh                = errors.New("block range too high")
	ErrNoWSConnection                   = errors.New("no websocket connection")
	ErrUnknownSubscriptionType          = errors.New("unknown subscription type")
)

// defaultTimeout is the timeout to remove the filters that don't have a web socket stream
var defaultTimeout = 1 * time.Minute

const (
	// The index in heap which is indicating the element is not in the heap
	NoIndexInHeap = -1
)

// subscriptionType determines which event type the filter is subscribed to
type subscriptionType byte

const (
	// Blocks represents subscription type for blockchain events
	Blocks subscriptionType = iota
	// PendingTransactions represents subscription type for tx pool events
	PendingTransactions
)

// filter is an interface that BlockFilter and LogFilter implement
type filter interface {
	// hasWSConn returns the flag indicating the filter has web socket stream
	hasWSConn() bool

	// getFilterBase returns filterBase that has common fields
	getFilterBase() *filterBase

	// getSubscriptionType returns the type of the event the filter is subscribed to
	getSubscriptionType() subscriptionType

	// getUpdates returns stored data in a JSON serializable form
	getUpdates() (interface{}, error)

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
	expiresAt time.Time

	// websocket connection
	ws wsConn
}

// newFilterBase initializes filterBase with unique ID
func newFilterBase(ws wsConn) filterBase {
	uuidObj := uuid.New()

	return filterBase{
		id:        string(encodeToHex(uuidObj[:])),
		ws:        ws,
		heapIndex: NoIndexInHeap,
	}
}

// getFilterBase returns its own reference so that child struct can return base
func (f *filterBase) getFilterBase() *filterBase {
	return f
}

// hasWSConn returns the flag indicating this filter has websocket connection
func (f *filterBase) hasWSConn() bool {
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
	if !f.hasWSConn() {
		return ErrNoWSConnection
	}

	return f.ws.WriteMessage(
		websocket.TextMessage,
		[]byte(fmt.Sprintf(ethSubscriptionTemplate, f.id, msg)),
	)
}

// blockFilter is a filter to store the updates of block
type blockFilter struct {
	filterBase
	sync.Mutex

	block *headElem
}

// takeBlockUpdates advances blocks from head to latest and returns header array
func (f *blockFilter) takeBlockUpdates() []*block {
	updates, newHead := f.block.getUpdates()
	f.setHeadElem(newHead)

	return updates
}

// setHeadElem sets the block filter head
func (f *blockFilter) setHeadElem(head *headElem) {
	f.Lock()
	defer f.Unlock()

	f.block = head
}

// getUpdates returns updates of blocks in string
func (f *blockFilter) getUpdates() (interface{}, error) {
	headers := f.takeBlockUpdates()

	updates := make([]string, len(headers))
	for index, header := range headers {
		updates[index] = header.Hash.String()
	}

	return updates, nil
}

// sendUpdates writes the updates of blocks to web socket stream
func (f *blockFilter) sendUpdates() error {
	updates := f.takeBlockUpdates()

	for _, header := range updates {
		raw, err := json.Marshal(header)
		if err != nil {
			return err
		}

		if err := f.writeMessageToWs(string(raw)); err != nil {
			return err
		}
	}

	return nil
}

// getSubscriptionType returns the type of the event the filter is subscribed to
func (f *blockFilter) getSubscriptionType() subscriptionType {
	return Blocks
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
	f.logs = []*Log{} // create brand-new slice so that prevent new logs from being added to current logs

	return logs
}

// getUpdates returns stored logs in string
func (f *logFilter) getUpdates() (interface{}, error) {
	logs := f.takeLogUpdates()

	return logs, nil
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

// getSubscriptionType returns the type of the event the filter is subscribed to
func (f *logFilter) getSubscriptionType() subscriptionType {
	return Blocks
}

// pendingTxFilter is a filter to store pending tx
type pendingTxFilter struct {
	filterBase
	sync.Mutex

	txHashes []string
}

// appendPendingTxHashes appends new pending tx hash to tx hashes
func (f *pendingTxFilter) appendPendingTxHashes(txHash string) {
	f.Lock()
	defer f.Unlock()

	f.txHashes = append(f.txHashes, txHash)
}

// takePendingTxsUpdates returns all saved pending tx hashes in filter and sets a new slice
func (f *pendingTxFilter) takePendingTxsUpdates() []string {
	f.Lock()
	defer f.Unlock()

	txHashes := f.txHashes
	f.txHashes = []string{}

	return txHashes
}

// getSubscriptionType returns the type of the event the filter is subscribed to
func (f *pendingTxFilter) getSubscriptionType() subscriptionType {
	return PendingTransactions
}

// getUpdates returns stored pending tx hashes
func (f *pendingTxFilter) getUpdates() (interface{}, error) {
	pendingTxHashes := f.takePendingTxsUpdates()

	return pendingTxHashes, nil
}

// sendUpdates write the hashes for all pending transactions to web socket stream
func (f *pendingTxFilter) sendUpdates() error {
	pendingTxHashes := f.takePendingTxsUpdates()

	for _, txHash := range pendingTxHashes {
		if err := f.writeMessageToWs(txHash); err != nil {
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

	// TxPoolSubscribe subscribes for tx pool events
	TxPoolSubscribe(request *proto.SubscribeRequest) (<-chan *proto.TxPoolEvent, func(), error)
}

// FilterManager manages all running filters
type FilterManager struct {
	sync.RWMutex

	logger hclog.Logger

	timeout time.Duration

	store           filterManagerStore
	subscription    blockchain.Subscription
	blockStream     *blockStream
	blockRangeLimit uint64

	filters  map[string]filter
	timeouts timeHeapImpl

	updateCh chan struct{}
	closeCh  chan struct{}
}

func NewFilterManager(logger hclog.Logger, store filterManagerStore, blockRangeLimit uint64) *FilterManager {
	m := &FilterManager{
		logger:          logger.Named("filter"),
		timeout:         defaultTimeout,
		store:           store,
		blockRangeLimit: blockRangeLimit,
		filters:         make(map[string]filter),
		timeouts:        timeHeapImpl{},
		updateCh:        make(chan struct{}),
		closeCh:         make(chan struct{}),
	}

	// start blockstream with the current header
	header := store.Header()

	block := toBlock(&types.Block{Header: header}, false)
	m.blockStream = newBlockStream(block)

	// start the head watcher
	m.subscription = store.SubscribeEvents()

	return m
}

// Run starts worker process to handle events
func (f *FilterManager) Run() {
	// watch for new events in the blockchain
	blockWatchCh := make(chan *blockchain.Event)

	go func() {
		for {
			evnt := f.subscription.GetEvent()
			if evnt == nil {
				return
			}
			blockWatchCh <- evnt
		}
	}()

	// watch for new events in the tx pool
	txRequest := &proto.SubscribeRequest{
		Types: []proto.EventType{proto.EventType_ADDED},
	}

	txWatchCh, txPoolUnsubscribe, err := f.store.TxPoolSubscribe(txRequest)
	if err != nil {
		f.logger.Error("Unable to subscribe to tx pool")

		return
	}

	defer txPoolUnsubscribe()

	var timeoutCh <-chan time.Time

	for {
		// check for the next filter to be removed
		filterID, filterExpiresAt := f.nextTimeoutFilter()

		// set timer to remove filter
		if filterID != "" {
			timeoutCh = time.After(time.Until(filterExpiresAt))
		}

		select {
		case evnt := <-blockWatchCh:
			// new blockchain event
			if err := f.dispatchEvent(evnt); err != nil {
				f.logger.Error("failed to dispatch block event", "err", err)
			}

		case evnt := <-txWatchCh:
			// new tx pool event
			if err := f.dispatchEvent(evnt); err != nil {
				f.logger.Error("failed to dispatch tx pool event", "err", err)
			}

		case <-timeoutCh:
			// timeout for filter
			// if filter still exists
			if !f.Uninstall(filterID) {
				f.logger.Warn("failed to uninstall filter", "id", filterID)
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
		block:      f.blockStream.getHead(),
	}

	if filter.hasWSConn() {
		ws.SetFilterID(filter.id)
	}

	return f.addFilter(filter)
}

// NewLogFilter adds new LogFilter
func (f *FilterManager) NewLogFilter(logQuery *LogQuery, ws wsConn) string {
	filter := &logFilter{
		filterBase: newFilterBase(ws),
		query:      logQuery,
	}

	if filter.hasWSConn() {
		ws.SetFilterID(filter.id)
	}

	return f.addFilter(filter)
}

// NewPendingTxFilter adds new PendingTxFilter
func (f *FilterManager) NewPendingTxFilter(ws wsConn) string {
	filter := &pendingTxFilter{
		filterBase: newFilterBase(ws),
		txHashes:   []string{},
	}

	if filter.hasWSConn() {
		ws.SetFilterID(filter.id)
	}

	return f.addFilter(filter)
}

// Exists checks the filter with given ID exists
func (f *FilterManager) Exists(id string) bool {
	f.RLock()
	defer f.RUnlock()

	_, ok := f.filters[id]

	return ok
}

func (f *FilterManager) getLogsFromBlock(query *LogQuery, block *types.Block) ([]*Log, error) {
	receipts, err := f.store.GetReceiptsByHash(block.Header.Hash)
	if err != nil {
		return nil, err
	}

	logIdx := uint64(0)
	logs := make([]*Log, 0)

	for idx, receipt := range receipts {
		for _, log := range receipt.Logs {
			if query.Match(log) {
				logs = append(logs, toLog(log, logIdx, uint64(idx), block.Header, block.Transactions[idx].Hash))
			}

			logIdx++
		}
	}

	return logs, nil
}

func (f *FilterManager) getLogsFromBlocks(query *LogQuery) ([]*Log, error) {
	from, err := GetNumericBlockNumber(query.fromBlock, f.store)
	if err != nil {
		return nil, err
	}

	to, err := GetNumericBlockNumber(query.toBlock, f.store)
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

	// if not disabled, avoid handling large block ranges
	if f.blockRangeLimit != 0 && to-from > f.blockRangeLimit {
		return nil, ErrBlockRangeTooHigh
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
		// BlockHash is set -> fetch logs from this block only
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

	// gets logs from a range of blocks
	return f.getLogsFromBlocks(query)
}

// getFilterByID fetches the filter by the ID
func (f *FilterManager) getFilterByID(filterID string) filter {
	f.RLock()
	defer f.RUnlock()

	return f.filters[filterID]
}

// GetLogFilterFromID return log filter for given filterID
func (f *FilterManager) GetLogFilterFromID(filterID string) (*logFilter, error) {
	filterRaw := f.getFilterByID(filterID)

	if filterRaw == nil {
		return nil, ErrFilterNotFound
	}

	logFilter, ok := filterRaw.(*logFilter)
	if !ok {
		return nil, ErrCastingFilterToLogFilter
	}

	return logFilter, nil
}

// GetFilterChanges returns the updates of the filter with given ID in string, and refreshes the timeout on the filter
func (f *FilterManager) GetFilterChanges(id string) (interface{}, error) {
	filter, res, err := f.getFilterAndChanges(id)

	if err == nil && !filter.hasWSConn() {
		// Refresh the timeout on this filter
		f.Lock()
		f.refreshFilterTimeout(filter.getFilterBase())
		f.Unlock()
	}

	return res, err
}

// getFilterAndChanges returns the updates of the filter with given ID in string (read lock only)
func (f *FilterManager) getFilterAndChanges(id string) (filter, interface{}, error) {
	f.RLock()
	defer f.RUnlock()

	filter, ok := f.filters[id]

	if !ok {
		return nil, nil, ErrFilterNotFound
	}

	// we cannot get updates from a ws filter with getFilterChanges
	if filter.hasWSConn() {
		return nil, nil, ErrWSFilterDoesNotSupportGetChanges
	}

	res, err := filter.getUpdates()
	if err != nil {
		return nil, nil, err
	}

	return filter, res, nil
}

// Uninstall removes the filter with given ID from list
func (f *FilterManager) Uninstall(id string) bool {
	f.Lock()
	defer f.Unlock()

	return f.removeFilterByID(id)
}

// removeFilterByID removes the filter with given ID [NOT Thread Safe]
func (f *FilterManager) removeFilterByID(id string) bool {
	// Make sure filter exists
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

// RemoveFilterByWs removes the filter with given WS [Thread safe]
func (f *FilterManager) RemoveFilterByWs(ws wsConn) {
	f.Lock()
	defer f.Unlock()

	f.removeFilterByID(ws.GetFilterID())
}

// refreshFilterTimeout updates the timeout for a filter to the current time
func (f *FilterManager) refreshFilterTimeout(filter *filterBase) {
	f.timeouts.removeFilter(filter)
	f.addFilterTimeout(filter)
}

// addFilterTimeout set timeout and add to heap
func (f *FilterManager) addFilterTimeout(filter *filterBase) {
	filter.expiresAt = time.Now().UTC().Add(f.timeout)
	f.timeouts.addFilter(filter)
	f.emitSignalToUpdateCh()
}

// addFilter is an internal method to add given filter to list and heap
func (f *FilterManager) addFilter(filter filter) string {
	f.Lock()
	defer f.Unlock()

	base := filter.getFilterBase()

	f.filters[base.id] = filter

	// Set timeout and add to heap if filter doesn't have web socket connection
	if !filter.hasWSConn() {
		f.addFilterTimeout(base)
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
func (f *FilterManager) nextTimeoutFilter() (string, time.Time) {
	f.RLock()
	defer f.RUnlock()

	if len(f.timeouts) == 0 {
		return "", time.Time{}
	}

	// peek the first item
	base := f.timeouts[0]

	return base.id, base.expiresAt
}

// dispatchEvent is an event handler for new block event
func (f *FilterManager) dispatchEvent(evnt interface{}) error {
	var subType subscriptionType

	// store new event in each filters
	switch evt := evnt.(type) {
	case *blockchain.Event:
		f.processBlockEvent(evt)

		subType = Blocks
	case *proto.TxPoolEvent:
		f.processTxEvent(evt)

		subType = PendingTransactions

	default:
		return ErrUnknownSubscriptionType
	}

	// send data to web socket stream
	if err := f.flushWsFilters(subType); err != nil {
		return err
	}

	return nil
}

// processEvent makes each filter append the new data that interests them
func (f *FilterManager) processBlockEvent(evnt *blockchain.Event) {
	f.RLock()
	defer f.RUnlock()

	for _, header := range evnt.NewChain {
		block := toBlock(&types.Block{Header: header}, false)

		// first include all the new headers in the blockstream for BlockFilter
		f.blockStream.push(block)

		// process new chain to include new logs for LogFilter
		if processErr := f.appendLogsToFilters(block); processErr != nil {
			f.logger.Error(fmt.Sprintf("Unable to process block, %v", processErr))
		}
	}
}

// appendLogsToFilters makes each LogFilters append logs in the header
func (f *FilterManager) appendLogsToFilters(header *block) error {
	receipts, err := f.store.GetReceiptsByHash(header.Hash)
	if err != nil {
		return err
	}

	// Get logFilters from filters
	logFilters := make([]*logFilter, 0)

	for _, f := range f.filters {
		if logFilter, ok := f.(*logFilter); ok {
			logFilters = append(logFilters, logFilter)
		}
	}

	if len(logFilters) == 0 {
		return nil
	}

	block, ok := f.store.GetBlockByHash(header.Hash, true)
	if !ok {
		f.logger.Error("could not find block in store", "hash", header.Hash.String())

		return nil
	}

	logIndex := uint64(0)

	for indx, receipt := range receipts {
		if receipt.TxHash == types.ZeroHash {
			// Extract tx Hash
			receipt.TxHash = block.Transactions[indx].Hash
		}
		// check the logs with the filters
		for _, log := range receipt.Logs {
			for _, f := range logFilters {
				if f.query.Match(log) {
					f.appendLog(toLog(log, logIndex, uint64(indx), block.Header, receipt.TxHash))
				}
			}

			logIndex++
		}
	}

	return nil
}

// processTxEvent makes each filter refresh the pending tx hashes
func (f *FilterManager) processTxEvent(evnt *proto.TxPoolEvent) {
	f.RLock()
	defer f.RUnlock()

	for _, f := range f.filters {
		if txFilter, ok := f.(*pendingTxFilter); ok {
			txFilter.appendPendingTxHashes(evnt.TxHash)
		}
	}
}

// flushWsFilters make each filters with web socket connection write the updates to web socket stream
// flushWsFilters also removes the filters if flushWsFilters notices the connection is closed
func (f *FilterManager) flushWsFilters(subType subscriptionType) error {
	closedFilterIDs := make([]string, 0)

	f.RLock()

	for id, filter := range f.filters {
		if !filter.hasWSConn() || filter.getSubscriptionType() != subType {
			continue
		}

		if flushErr := filter.sendUpdates(); flushErr != nil {
			// mark as closed if the connection is closed
			if errors.Is(flushErr, websocket.ErrCloseSent) || errors.Is(flushErr, net.ErrClosed) {
				closedFilterIDs = append(closedFilterIDs, id)

				f.logger.Warn(fmt.Sprintf("Subscription %s has been closed", id))

				continue
			}

			f.logger.Error(fmt.Sprintf("Unable to process flush, %v", flushErr))
		}
	}

	f.RUnlock()

	// remove filters with closed web socket connections from FilterManager
	if len(closedFilterIDs) > 0 {
		f.Lock()
		for _, id := range closedFilterIDs {
			f.removeFilterByID(id)
		}
		f.Unlock()

		f.logger.Info(fmt.Sprintf("Removed %d filters due to closed connections", len(closedFilterIDs)))
	}

	return nil
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
	return t[i].expiresAt.Before(t[j].expiresAt)
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
	head atomic.Value
}

func newBlockStream(head *block) *blockStream {
	b := &blockStream{}
	b.head.Store(&headElem{header: head})

	return b
}

func (b *blockStream) getHead() *headElem {
	head, _ := b.head.Load().(*headElem)

	return head
}

func (b *blockStream) push(header *block) {
	b.lock.Lock()
	defer b.lock.Unlock()

	newHead := &headElem{
		header: header.Copy(),
	}

	oldHead, _ := b.head.Load().(*headElem)
	oldHead.next.Store(newHead)

	b.head.Store(newHead)
}

type headElem struct {
	header *block
	next   atomic.Value
}

func (h *headElem) getUpdates() ([]*block, *headElem) {
	res := make([]*block, 0)
	cur := h

	for {
		next := cur.next.Load()
		if next == nil {
			// no more messages
			break
		} else {
			nextElem, _ := next.(*headElem)

			if nextElem.header != nil {
				res = append(res, nextElem.header)
			}
			cur = nextElem
		}
	}

	return res, cur
}
