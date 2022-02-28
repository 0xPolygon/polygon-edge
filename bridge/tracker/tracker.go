package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/0xPolygon/polygon-edge/blockchain/storage"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/go-web3"
	"math/big"
)

type Config struct {
	Confirmations uint64
	RootchainWS   string
	DBPath        string
	ContractABIs  map[types.Address][]string
}

//	cancellable context for tracker's listening mechanism
type contextSubscription struct {
	context context.Context
	cancel  context.CancelFunc
}

//	done returns the contexts Done channel
func (c *contextSubscription) done() <-chan struct{} {
	return c.context.Done()
}

//	Tracker represents an event listener that notifies
//	for each event emitted by a smart contract on the rootchain.
//	Events are collected from blocks at the desired depth (confirmations)
type Tracker struct {
	logger hclog.Logger

	// required block confirmations
	confirmations *big.Int

	// event channel
	eventCh chan []byte

	// cancel subscription
	ctxSubscription contextSubscription

	//	rootchain subscription object
	sub subscription

	//	rootchain client
	client *rootchainClient

	//	db for last processed block number
	db storage.Storage

	//	events that tracker will listen for
	contracts []*contractABI
}

//	NewEventTracker returns a new tracker object
func NewEventTracker(logger hclog.Logger, config *Config) (*Tracker, error) {
	if config == nil {
		return nil, errors.New("no config provided")
	}

	//	create tracker
	tracker := &Tracker{
		logger:        logger.Named("event_tracker"),
		confirmations: big.NewInt(0).SetUint64(config.Confirmations),
		eventCh:       make(chan []byte),
	}

	var err error

	//	load abi events
	if config.ContractABIs != nil {
		tracker.contracts = loadABIs(config.ContractABIs)
	}

	//	load db (last processed block number)
	if tracker.db, err = initRootchainDB(logger, config.DBPath); err != nil {
		logger.Error("cannot initialize db", "errCh", err)

		return nil, err
	}

	//	create rootchain client
	if tracker.client, err = newRootchainClient(config.RootchainWS); err != nil {
		logger.Error("cannot connect to rootchain", "errCh", err)

		return nil, err
	}

	return tracker, nil
}

//	Start initializes context for the tracking mechanism
//	and starts listening for events on the rootchain
func (t *Tracker) Start() error {
	//	subscribe for new headers
	if err := t.subscribeToRootchain(); err != nil {
		t.logger.Error("cannot subscribe to rootchain", "errCh", err)

		return err
	}

	//	start processing new header events
	go t.startEventTracking()

	return nil
}

//	Stop stops the tracker's listening mechanism
func (t *Tracker) Stop() error {
	//	stop subscription
	t.ctxSubscription.cancel()

	//	close db
	t.db.Close()

	//	close rootchain client
	if err := t.client.close(); err != nil {
		t.logger.Error("cannot close rootchain client", "errCh", err)

		return err
	}

	return nil
}

//	getEventChannel returns the tracker's event channel
func (t *Tracker) GetEventChannel() <-chan []byte {
	return t.eventCh
}

//	subscribeToRootchain subscribes the tracker for new
//	header events on the rootchain
func (t *Tracker) subscribeToRootchain() error {
	//	create cancellable context for tracker
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.ctxSubscription = contextSubscription{
		context: ctx,
		cancel:  cancelFunc,
	}

	//	create subscription for new header events
	var err error
	if t.sub, err = t.client.subscribeNewHeads(); err != nil {
		return err
	}

	return nil
}

//	startEventTracking handles the subscription object (provided by the rootchain client)
func (t *Tracker) startEventTracking() {
	for {
		select {
		//	process new header
		case header := <-t.sub.newHeadCh():
			t.trackHeader(header)

		//	handle sub error
		case err := <-t.sub.errCh():
			t.ctxSubscription.cancel()
			t.logger.Error("subscription cancelled", err)

			return

		//	stop tracker's sub
		case <-t.ctxSubscription.done():
			if err := t.sub.unsubscribe(); err != nil {
				t.logger.Error("cannot unsubscribe", "errCh", err)

				return
			}

			t.logger.Debug("subscription stopped")

			return
		}
	}
}

//	trackHeader determines the range of block to query
//	based on the blockNumber of the header and issues an appropriate
//	eth_getLogs call. If an event of interest was emitted,
//	it is sent to eventCh
func (t *Tracker) trackHeader(header *ethHeader) {
	//	determine range of blocks to query
	fromBlock, toBlock := t.calculateRange(header)
	if fromBlock == nil || toBlock == nil {
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
//	to query for events
func (t *Tracker) calculateRange(header *ethHeader) (from, to *big.Int) {
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

	return fromBlock, toBlock
}

//	loadLastBlock returns the block number of the last
// 	block processed by the tracker (if available)
func (t *Tracker) loadLastBlock() *big.Int {
	lastBlockNumber, ok := t.db.ReadHeadNumber()
	if !ok {
		return nil
	}

	return big.NewInt(0).SetUint64(lastBlockNumber)
}

//	saveLastBlock stores the number of the last block processed
//	into the tracker's database
func (t *Tracker) saveLastBlock(blockNumber *big.Int) error {
	return t.db.WriteHeadNumber(blockNumber.Uint64())
}

//	queryEvents collects all events on the rootchain that occurred
//	between blocks fromBlock and toBlock (inclusive)
func (t *Tracker) queryEvents(fromBlock, toBlock *big.Int) []*web3.Log {
	//	create the query filter
	queryFilter := setupQueryFilter(fromBlock, toBlock, t.contracts)

	//	call eth_getLogs
	logs, err := t.client.getLogs(queryFilter)
	if err != nil {
		t.logger.Error("eth_getLogs failed", "errCh", err)

		return nil
	}

	//	overwrite checkpoint
	if err := t.saveLastBlock(toBlock); err != nil {
		t.logger.Error("cannot save last block number proccesed", "errCh", err)
	}

	return logs
}

//	notify sends the given logs to the event channel
func (t *Tracker) notify(logs ...*web3.Log) {
	for _, log := range logs {
		bytesLog, err := json.Marshal(log)
		if err != nil {
			t.logger.Error("cannot marshal log", "errCh", err)
		}

		// notify
		select {
		case t.eventCh <- bytesLog:
		default:
		}
	}
}
