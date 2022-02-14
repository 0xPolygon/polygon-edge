package tracker

import (
	"context"
	"encoding/json"
	"math/big"
	"os"

	"github.com/0xPolygon/polygon-edge/types"

	"github.com/hashicorp/go-hclog"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/umbracle/go-web3"
)

const (
	lastQueriedBlockNumber = "last-block-num" //	db key
)

//	Tracker represents an event listener that notifies
//	for each event emitted on the rootchain (ethereum).
//	Events of interest are defined in rootchain.go
type Tracker struct {
	logger hclog.Logger

	// required block confirmations
	confirmations uint64

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
		confirmations: confirmations,
		headerCh:      make(chan *ethHeader, 1),
		eventCh:       make(chan []byte),
	}

	//	create rootchain client
	client, err := newRootchainClient(rootchainWS)
	if err != nil {
		logger.Error("cannot connect to rootchain", "err", err)

		return nil, err
	}

	tracker.client = client

	//	load db (last processed block number)
	db, err := tracker.loadDB()
	if err != nil {
		logger.Error("cannot load db", "err", err)

		return nil, err
	}

	tracker.db = db

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

//	loadDB creates a new database (or loads existing)
//	for storing the last processed block's number by the tracker.
func (t *Tracker) loadDB() (*leveldb.DB, error) {
	//	get path
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	//	create or load db
	db, err := leveldb.OpenFile(cwd+"/event_tracker/last_block_number", nil)
	if err != nil {
		return nil, err
	}

	return db, nil
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
	confirmations := big.NewInt(0).SetUint64(t.confirmations)
	if latestHeight.Cmp(confirmations) < 0 {
		//	block height less than required
		t.logger.Debug(
			"not enough confirmations",
			"current", latestHeight.Uint64(),
			"required", confirmations.Uint64())

		return
	}

	/*	right bound */
	toBlock := big.NewInt(0).Sub(latestHeight, confirmations)

	/*	left bound */
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

/* Header structs parsed by the client */

//	Ethereum header
//type ethHeader struct {
//	Difficulty   string        `json:"difficulty"`
//	ExtraData    string        `json:"extraData"`
//	GasLimit     string        `json:"gasLimit"`
//	GasUsed      string        `json:"gasUsed"`
//	LogsBloom    types.Bloom   `json:"logsBloom"`
//	Miner        types.Address `json:"miner"`
//	Nonce        string        `json:"nonce"`
//	Number       string        `json:"number"`
//	ParentHash   types.Hash    `json:"parentHash"`
//	ReceiptsRoot types.Hash    `json:"receiptsRoot"`
//	Sha3Uncles   types.Hash    `json:"sha3Uncles"`
//	StateRoot    types.Hash    `json:"stateRoot"`
//	Timestamp    string        `json:"timestamp"`
//	TxRoot       types.Hash    `json:"transactionsRoot"`
//	MixHash      types.Hash    `json:"mixHash"`
//	Hash         types.Hash    `json:"hash"`
//}

//	Polygon-Edge header
type ethHeader struct {
	Difficulty   uint64        `json:"difficulty"`
	ExtraData    string        `json:"extraData"`
	GasLimit     uint64        `json:"gasLimit"`
	GasUsed      uint64        `json:"gasUsed"`
	LogsBloom    types.Bloom   `json:"logsBloom"`
	Miner        types.Address `json:"miner"`
	Nonce        string        `json:"nonce"`
	Number       uint64        `json:"number"`
	ParentHash   types.Hash    `json:"parentHash"`
	ReceiptsRoot types.Hash    `json:"receiptsRoot"`
	Sha3Uncles   types.Hash    `json:"sha3Uncles"`
	StateRoot    types.Hash    `json:"stateRoot"`
	Timestamp    uint64        `json:"timestamp"`
	TxRoot       types.Hash    `json:"transactionsRoot"`
	MixHash      types.Hash    `json:"mixHash"`
	Hash         types.Hash    `json:"hash"`
}
