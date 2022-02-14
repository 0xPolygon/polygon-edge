package tracker

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/hashicorp/go-hclog"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/umbracle/go-web3"
)

const (
	//	required block depth for fetching events on the rootchain
	BlockConfirmations = 6

	//	db key for saving tracker's progress (chain height)
	lastQueriedBlockNumber = "last-block-num"
)

//	Tracker represents an event listener that notifies
//	for each event emitted on the rootchain.
//	Events of interest are defined in rootchain.go
type Tracker struct {
	logger hclog.Logger

	// required block confirmations
	confirmations *big.Int

	// newHeads subscription channel
	headerCh chan *ethHeader

	// event channel
	eventCh chan []byte

	// cancel funcs
	cancelSubscription, cancelEventTracking context.CancelFunc

	//	rootchain client
	client *rootchainClient

	//	db for last processed block number
	db *leveldb.DB
}

//	NewEventTracker returns a new tracker able to listen for events
//	at the desired block depth. Events of interest are defined
//	in rootchain.go
func NewEventTracker(logger hclog.Logger, confirmations uint64) (*Tracker, error) {
	tracker := &Tracker{
		logger:        logger.Named("event_tracker"),
		confirmations: big.NewInt(0).SetUint64(confirmations),
		headerCh:      make(chan *ethHeader, 1),
		eventCh:       make(chan []byte),
	}

	var err error

	//	create rootchain client
	if tracker.client, err = newRootchainClient(rootchainWS); err != nil {
		logger.Error("cannot connect to rootchain", "err", err)

		return nil, err
	}

	//	load db (last processed block number)
	if tracker.db, err = initRootchainDB(); err != nil {
		logger.Error("cannot initialize db", "err", err)

		return nil, err
	}

	return tracker, nil
}

//	Start runs the tracker's listening mechanism:
//	1. startEventTracking - goroutine keeping track of
//	the last block queried, as well as collecting events
//	of interest.
//
//	2. startSubscription - goroutine responsible for handling
//	the subscription object. This object is provided by the
//	client's subscribeNewHeads method that initiates the subscription.
func (t *Tracker) Start() error {
	//	create cancellable contexts
	ctxSubscription, cancelSubscription := context.WithCancel(context.Background())
	t.cancelSubscription = cancelSubscription

	ctxEventTracking, cancelEventTracking := context.WithCancel(context.Background())
	t.cancelEventTracking = cancelEventTracking

	//	start event tracking process early
	go t.startEventTracking(ctxEventTracking)

	//	create newHeads subscription
	subscription, err := t.client.subscribeNewHeads(t.headerCh)
	if err != nil {
		t.logger.Error("cannot subscribe to rootchain", "err", err)

		return err
	}

	//	start receiving new header events
	go t.startSubscription(ctxSubscription, subscription)

	return nil
}

//	Stop stops the tracker's listening mechanism.
func (t *Tracker) Stop() error {
	//	stop subscription
	t.cancelSubscription()

	// 	stop processing headers
	t.cancelEventTracking()

	//	close rootchain client
	if err := t.client.close(); err != nil {
		t.logger.Error("cannot close rootchain client", "err", err)

		return err
	}

	return nil
}

//	getEventChannel returns the tracker's event channel.
func (t *Tracker) getEventChannel() <-chan []byte {
	return t.eventCh
}

//	startSubscription handles the subscription object (provided by the rootchain client).
func (t *Tracker) startSubscription(ctx context.Context, sub subscription) {
	for {
		select {
		case err := <-sub.err():
			t.logger.Error("subscription error", err)

			t.cancelSubscription()
			t.logger.Debug("cancelling subscription")

			return
		case <-ctx.Done():
			if err := sub.unsubscribe(); err != nil {
				t.logger.Error("cannot unsubscribe", "err", err)

				return
			}

			t.logger.Debug("subscription stopped")

			return
		}
	}
}

//	startEventTracking tracks each header (received by the subscription) for events.
func (t *Tracker) startEventTracking(ctx context.Context) {
	for {
		select {
		case header := <-t.headerCh:
			t.trackHeader(header)
		case <-ctx.Done():
			t.logger.Debug("stopping header process")

			return
		}
	}
}

//	trackHeader determines the range of block to query
//	based on the blockNumber of the header and issues an appropriate
//	eth_getLogs call. If an event of interest was emitted,
//	it is sent to eventCh.
func (t *Tracker) trackHeader(header *ethHeader) {
	//	determine range of blocks to query
	fromBlock, toBlock, ok := t.calculateRange(header)
	if !ok {
		//	we are returning here because the calculated
		//	range was already queried or the chain is not
		//	at the desired depth.
		return
	}

	t.logger.Info("querying events within block range", "from", fromBlock, "to", toBlock)

	//	fetch logs
	logs := t.queryEvents(fromBlock, toBlock)
	t.logger.Info("matched events", "num", len(logs))

	//	notify each matched log event
	t.notify(logs...)
}

//	calculateRange determines the next range of blocks
//	to query for events.
func (t *Tracker) calculateRange(header *ethHeader) (from, to uint64, ok bool) {
	//	extract block number from header field
	//	TODO: polygon-edge header
	latestHeight := big.NewInt(0).SetUint64(header.Number)

	//	or: TODO ethereum header
	//latestHeight, _ := types.ParseUint256orHex(&header.Number)

	//	check if block number is at required depth
	if latestHeight.Cmp(t.confirmations) < 0 {
		//	block height less than required
		t.logger.Debug(
			"not enough confirmations",
			"current", latestHeight.Uint64(),
			"required", t.confirmations.Uint64())

		return
	}

	//	right bound
	toBlock := big.NewInt(0).Sub(latestHeight, t.confirmations)

	//	left bound
	fromBlock := t.loadLastBlock()
	if fromBlock == nil {
		fromBlock = toBlock
	} else {
		//	increment - last block number was already queried
		fromBlock = fromBlock.Add(fromBlock, big.NewInt(1))
	}

	if toBlock.Cmp(fromBlock) < 0 {
		//	no need to perform any query
		//	as left bound is higher than the right
		return
	}

	return fromBlock.Uint64(), toBlock.Uint64(), true
}

//	loadLastBlock returns the block number of the last
// 	block processed by the tracker (if available).
func (t *Tracker) loadLastBlock() *big.Int {
	//	read from db
	bytesLastBlock, err := t.db.Get([]byte(lastQueriedBlockNumber), nil)
	if err != nil {
		t.logger.Error("cannot read db", "err", err)

		return nil
	}

	//	parse
	lastBlock, ok := big.NewInt(0).SetString(string(bytesLastBlock), 10)
	if !ok {
		return nil
	}

	return lastBlock
}

//	saveLastBlock stores the number of the last block processed
//	into the tracker's database.	.
func (t *Tracker) saveLastBlock(blockNumber *big.Int) error {
	//	store last processed block number
	if err := t.db.Put(
		[]byte(lastQueriedBlockNumber),
		[]byte(blockNumber.String()),
		nil,
	); err != nil {
		return err
	}

	return nil
}

//	queryEvents collects all events on the rootchain that occurred
//	between blocks fromBlock and toBlock (inclusive).
func (t *Tracker) queryEvents(fromBlock, toBlock uint64) []*web3.Log {
	queryFilter := &web3.LogFilter{}

	//	set range of blocks to query
	queryFilter.SetFromUint64(fromBlock)
	queryFilter.SetToUint64(toBlock)

	//	set smart contract addresses
	queryFilter.Address = []web3.Address{
		web3.HexToAddress(PoCSC),
		web3.HexToAddress(AnotherEventSC),
		web3.HexToAddress(ThirdEventSC),
	}

	//	set relevant topics (defined in contracts)
	queryFilter.Topics = [][]*web3.Hash{
		{
			&topicPoCEvent,
			&topicAnotherEvent,
		},
	}

	//	call eth_getLogs
	logs, err := t.client.getLogs(queryFilter)
	if err != nil {
		t.logger.Error("eth_getLogs failed", "err", err)

		return nil
	}

	//	overwrite checkpoint
	if err := t.saveLastBlock(big.NewInt(0).SetUint64(toBlock)); err != nil {
		t.logger.Error("cannot save last block number proccesed", "err", err)
	}

	return logs
}

//	notify sends the given logs to the event channel.
func (t *Tracker) notify(logs ...*web3.Log) {
	for _, log := range logs {
		bytesLog, err := json.Marshal(log)
		if err != nil {
			t.logger.Error("cannot marshal log", "err", err)
		}

		// notify
		select {
		case t.eventCh <- bytesLog:
		default:
		}
	}
}
