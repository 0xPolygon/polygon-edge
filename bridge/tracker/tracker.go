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
	lastProcessedBlock = "last-processed-block" //	db key
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
	cancelSubscription, cancelHeaderProcess context.CancelFunc

	//	rootchain client
	client *rootchainClient

	//	db for last processed block number
	db *leveldb.DB
}

//	NewEventTracker returns a new tracker with the desired
//	confirmations depth. Events are defined in rootchain.go
func NewEventTracker(logger hclog.Logger, confirmations uint64) (*Tracker, error) {
	tracker := &Tracker{
		logger:        logger.Named("event_tracker"),
		confirmations: confirmations,
		headerCh:      make(chan *ethHeader, 1),
		eventCh:       make(chan []byte),
	}

	//	create rootchain rootchainClient
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
//	1. startHeaderProcess - goroutine processing each header
//		received by the subscription
//
//	2. startSubscription - goroutine that subscribes to the rootchain
//		for new headers, sending them to the header process
func (t *Tracker) Start() error {
	//	create cancellable contexts
	ctxSubscription, cancelSub := context.WithCancel(context.Background())
	t.cancelSubscription = cancelSub

	ctxHeaderProcess, cancelHeader := context.WithCancel(context.Background())
	t.cancelHeaderProcess = cancelHeader

	//	start header processing early
	go t.startHeaderProcess(ctxSubscription)

	//	create newHeads subscription
	subscription, err := t.client.subscribeNewHeads(t.headerCh)
	if err != nil {
		t.logger.Error("cannot subscribe to rootchain", "err", err)

		return err
	}

	//	start receiving new header events
	go t.startSubscription(ctxHeaderProcess, subscription)

	return nil
}

//	Stop stops the tracker's listening mechanism.
func (t *Tracker) Stop() error {
	//	stop subscription
	t.cancelSubscription()

	// 	stop processing headers
	t.cancelHeaderProcess()

	//	close rootchain client
	if err := t.client.close(); err != nil {
		t.logger.Error("cannot close rootchainClient", "err", err)

		return err
	}

	return nil
}

//	getEventChannel returns the tracker's event channel.
//func (t *Tracker) getEventChannel() <-chan []byte {
//	return t.eventCh
//}

//	startSubscription handles subscription errors or context cancel.
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

//	startHeaderProcess processes new header events received by the subscription.
func (t *Tracker) startHeaderProcess(ctx context.Context) {
	for {
		select {
		case header := <-t.headerCh:
			t.processHeader(header)
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

//	processHeader determines the range of block to query
//	based on the blockNumber of the header and issues a
//	eth_getLogs call. If an event of interest was emitted,
//	it is sent to eventCh.
func (t *Tracker) processHeader(header *ethHeader) {
	t.logger.Info("processing header", "num", header.Number)

	//	determine range of blocks to query
	fromBlock, toBlock := t.calculateRange(header)

	//	fetch logs
	logs := t.queryEvents(fromBlock, toBlock)

	t.logger.Info("matched events", "num", len(logs))

	//	notify each matched log event
	if err := t.notify(logs...); err != nil {
		t.logger.Error("failed to notify events", "err", err)
	}
}

func (t *Tracker) calculateRange(header *ethHeader) (from, to uint64) {
	//	extract block number from header field
	//	TODO: polygon-edge header
	latestHeight := big.NewInt(0).SetUint64(header.Number)

	//	or: TODO ethereum header
	//latestHeight, _ := types.ParseUint256orHex(&header.Number)

	//	check  if block number is at required depth
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
		//	increment - fetched block number was already queried
		fromBlock = fromBlock.Add(fromBlock, big.NewInt(1))
	}

	//	TODO: not sure how this will ever happen, but Heimdall checks this
	if toBlock.Cmp(fromBlock) < 0 {
		fromBlock = toBlock
	}

	return fromBlock.Uint64(), toBlock.Uint64()
}

//	loadLastBlock returns the block number of the last
// 	block processed by the tracker (if available).
func (t *Tracker) loadLastBlock() *big.Int {
	//	read from db
	bytesLastBlock, err := t.db.Get([]byte(lastProcessedBlock), nil)
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

//	saveLastBlock stores the number of the last block processed.
func (t *Tracker) saveLastBlock(blockNumber *big.Int) error {
	//	store last processed block number
	if err := t.db.Put(
		[]byte(lastProcessedBlock),
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

	//	fetch logs
	logs, err := t.client.getLogs(queryFilter)
	if err != nil {
		t.logger.Error("eth_getLogs failed", "err", err)

		return nil
	}

	t.logger.Debug("eth_getLogs", "from", fromBlock, "to", toBlock)

	//	overwrite checkpoint
	if err := t.saveLastBlock(big.NewInt(0).SetUint64(toBlock)); err != nil {
		t.logger.Error("cannot save last block number proccesed", "err", err)
	}

	return logs
}

//	notify sends the given logs to the event channel.
func (t *Tracker) notify(logs ...*web3.Log) error {
	for _, log := range logs {
		bytesLog, err := json.Marshal(log)
		if err != nil {
			t.logger.Error("cannot marshal log", "err", err)

			return err
		}

		// notify
		select {
		case t.eventCh <- bytesLog:
		default:
		}
	}

	return nil
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
