package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strconv"

	"github.com/0xPolygon/polygon-edge/jsonrpc"
	"github.com/0xPolygon/polygon-edge/types"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/umbracle/go-web3"
	rpc "github.com/umbracle/go-web3/jsonrpc"
)

const (

	//	ropsten
	//ethWS          = "wss://ropsten.infura.io/ws/v3/17eac086ff36442ebd43737400eb71ca"
	//ethHTTP        = "https://ropsten.infura.io/v3/58f8f6612b494cac85e2c8ab2ce11ed1"

	//	edge
	ethWS   = "ws://127.0.0.1:10002/ws"
	ethHTTP = "http://127.0.0.1:10002"

	lastProcessedBlock = "last-processed-block"
	stateSenderAddress = "74FbD47E7390E345982A3b7e413D35332945C10C"
)

type ethSubscribeRequest struct {
	JsonRPC string   `json:"jsonrpc"`
	Method  string   `json:"method"`
	Params  []string `json:"params"`
	Id      int      `json:"id"`
}

type ethSubscribeResponse struct {
	JsonRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  struct {
		Result       json.RawMessage `json:"result"`
		Subscription string          `json:"subscription"`
	} `json:"params"`
}

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

//	Edge header
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

//	Tracker represents an event listener that notifies
//	for each event emitted to the rootchain (ethereum - hardcoded).
//	Events of interest are defined in abi.go
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

	// websocket connection (newHeads)
	wsConn *websocket.Conn

	// rpc client (eth_getLogs)
	rpcClient *rpc.Client

	//	db (last processed block)
	db *leveldb.DB
}

//	NewEventTracker returns a new tracker with the desired
//	confirmations depth. Events are defined in abi.go
func NewEventTracker(logger hclog.Logger, confirmations uint64) (*Tracker, error) {
	tracker := &Tracker{
		logger:        logger.Named("event_tracker"),
		confirmations: confirmations,
		headerCh:      make(chan *ethHeader),
		eventCh:       make(chan []byte),
	}

	//	load database for last processed block
	if err := tracker.loadDB(); err != nil {
		return nil, err
	}

	return tracker, nil
}

//	loadDB creates a new database (or loads existing)
//	for storing the last block processed by the tracker.
func (t *Tracker) loadDB() error {
	//	get path
	cwd, err := os.Getwd()
	if err != nil {
		t.logger.Error(
			"failed to create tracker",
			"err", err)

		return err
	}

	//	create or load db
	db, err := leveldb.OpenFile(
		cwd+"/event_tracker/last_block_number",
		nil,
	)
	if err != nil {
		t.logger.Error(
			"failed to load db",
			"err", err)

		return err
	}

	//	store db
	t.db = db

	return nil
}

//	Start connects the tracker to the rootchain
//	and listens for new events.
func (t *Tracker) Start() error {
	//	connect to the rootchain
	if err := t.connect(); err != nil {
		t.logger.Error(
			"could not connect",
			"err", err)

		return err
	}

	//	create cancellable contexts
	ctxSubscription, cancelSub := context.WithCancel(context.Background())
	t.cancelSubscription = cancelSub

	ctxHeaderProcess, cancelHeader := context.WithCancel(context.Background())
	t.cancelHeaderProcess = cancelHeader

	//	start the header processing early
	go t.startHeaderProcess(ctxSubscription)

	//	start receiving new heads
	go t.startSubscription(ctxHeaderProcess)

	return nil
}

//	getEventChannel returns the tracker's event channel.
func (t *Tracker) getEventChannel() <-chan []byte {
	return t.eventCh
}

func (t *Tracker) Stop() error {
	//	stop subscription
	t.cancelSubscription()

	// 	stop processing headers
	t.cancelHeaderProcess()

	//	disconnect rpc
	if err := t.rpcClient.Close(); err != nil {
		return err
	}

	return nil
}

//	connect connects the tracker to http and ws endpoints.
func (t *Tracker) connect() error {
	// create ws connection
	conn, _, err := websocket.DefaultDialer.Dial(
		ethWS,
		nil)
	if err != nil {
		t.logger.Error(
			"cannot connect to websocket",
			"err", err)

		return err
	}

	t.wsConn = conn
	t.logger.Debug("connected to ws endpoint")

	//	create http connection
	rpcClient, err := rpc.NewClient(ethHTTP)
	if err != nil {
		t.logger.Error("cannot connect to http")
		return err
	}

	t.rpcClient = rpcClient
	t.logger.Debug("connected to http endpoint")

	return nil
}

//	startSubscription subscribes to the rootchain for new headers.
//	Each header received is processed in startHeaderProcess().
func (t *Tracker) startSubscription(ctx context.Context) {
	//	subscribe for new headers
	if err := t.subscribeNewHeads(); err != nil {
		t.logger.Error(
			"could not subscribe",
			"err", err)

		return
	}

	defer func() {
		t.wsConn.Close()
		t.logger.Debug("closed websocket connection")
	}()

	//	listen for ws messages
	for {
		select {
		case <-ctx.Done():
			t.logger.Debug("stopping subscription")
			return

		default:
			//	read the subscription message
			res := ethSubscribeResponse{}
			if err := t.wsConn.ReadJSON(&res); err != nil {
				t.logger.Error(
					"cannot read message from ws",
					"err", err)

				continue
			}

			//	parse the result
			header := &ethHeader{}
			if err := json.Unmarshal(res.Params.Result, header); err != nil {
				t.logger.Error(
					"cannot parse message",
					"err", err)

				continue
			}

			//	send header for processing
			t.headerCh <- header
		}
	}
}

//	startHeaderProcess processes each header received by startSubscription().
func (t *Tracker) startHeaderProcess(ctx context.Context) {
	for {
		select {
		case header := <-t.headerCh:
			t.processHeader(header)
		case <-ctx.Done():
			t.logger.Debug("stopping header processing")
			return
		}
	}
}

func (t *Tracker) processHeader(header *ethHeader) {
	t.logger.Info("processing header", "num", header.Number)

	//	determine range of blocks to query
	fromBlock, toBlock := t.calculateRange(header)

	//	fetch logs
	logs := t.queryEvents(fromBlock, toBlock)

	t.matchAndDispatch(logs)

}

func (t *Tracker) matchAndDispatch(logs []*web3.Log) {
	if len(logs) == 0 {
		return
	}

	//	match each log with defined abi events
	for _, log := range logs {
		if NewRegistrationEvent.Match(log) {
			t.logger.Info("NewRegistrationEvent")

			if err := t.notify(log); err != nil {
				t.logger.Error(
					"cannot marshal log",
					"err", err)
			}

			continue
		}

		if RegistrationUpdatedEvent.Match(log) {
			t.logger.Info("RegistrationUpdatedEvent")

			if err := t.notify(log); err != nil {
				t.logger.Error(
					"cannot marshal log",
					"err", err)
			}

			continue
		}

		if StateSyncedEvent.Match(log) {
			t.logger.Info("StateSyncedEvent")

			if err := t.notify(log); err != nil {
				t.logger.Error(
					"cannot marshal log",
					"err", err)
			}

			continue
		}

		if PoCEvent.Match(log) {
			t.logger.Info("PoC event", "contract address", log.Address.String())

			if err := t.notify(log); err != nil {
				t.logger.Error(
					"cannot marshal log",
					"err", err)
			}

			continue
		}

		if TransferEvent.Match(log) {
			t.logger.Info("TransferEvent")

			if err := t.notify(log); err != nil {
				t.logger.Error(
					"cannot marshal log",
					"err", err)
			}

			continue
		}
	}
}

//	notify sends the given log to the event channel.
func (t *Tracker) notify(log *web3.Log) error {
	bLog, err := json.Marshal(log)
	if err != nil {
		t.logger.Error(
			"cannot marshal log",
			"err", err)

		return err
	}

	// notify
	t.eventCh <- bLog

	return nil
}

func (t *Tracker) calculateRange(header *ethHeader) (from, to uint64) {
	//	extract block number from header field
	latestHeight := big.NewInt(0).SetUint64(header.Number)
	//latestHeight, err := types.ParseUint256orHex(&header.Number)	TODO

	confirmationBlocks := big.NewInt(0).SetUint64(t.confirmations)
	if latestHeight.Cmp(confirmationBlocks) < 0 {
		//	block height less than required
		t.logger.Debug(
			"not enough confirmations",
			"current", latestHeight.Uint64(),
			"required", confirmationBlocks.Uint64())

		return
	}

	//	right bound (latest - confirmations)
	toBlock := big.NewInt(0).Sub(latestHeight, confirmationBlocks)

	//	left bound
	var fromBlock *big.Int

	lastBlock, ok := t.loadLastBlock()
	if !ok {
		// 	lastBlock not available
		fromBlock = toBlock
	} else {
		//	increment
		fromBlock = lastBlock.Add(
			lastBlock,
			big.NewInt(1))
	}

	return fromBlock.Uint64(), lastBlock.Uint64()
}

//	loadLastBlock returns the block number of the last
// 	block processed (if available).
func (t *Tracker) loadLastBlock() (*big.Int, bool) {
	//	check if db has last block
	if exists, _ := t.db.Has(
		[]byte(lastProcessedBlock),
		nil,
	); !exists {
		return nil, false
	}

	//	read from db
	bytesLastBlock, err := t.db.Get(
		[]byte(lastProcessedBlock),
		nil)
	if err != nil {
		//	log (cannot get db)
		t.logger.Error(
			"cannot read db",
			"err", err)

		return nil, false
	}

	//	parse
	lastBlock, ok := big.NewInt(0).SetString(
		string(bytesLastBlock),
		10)
	if !ok {
		return nil, false
	}

	return lastBlock, true

}

func (t *Tracker) queryEvents(fromBlock, toBlock uint64) []*web3.Log {
	queryFilter := &web3.LogFilter{
		Address: []web3.Address{
			web3.HexToAddress(stateSenderAddress),
		},
		Topics: nil,
	}

	queryFilter.SetFromUint64(fromBlock)
	queryFilter.SetToUint64(toBlock)

	fmt.Printf("eth_getLogs (from -> to): %d %d\n", fromBlock, toBlock)
	logs, err := t.rpcClient.Eth().GetLogs(queryFilter)
	if err != nil {
		//	eth_getLogs err
		println("eth_getLogs failed", err.Error())
		return nil
	}

	//	store last processed block number
	if err := t.db.Put(
		[]byte(lastProcessedBlock),
		[]byte(strconv.FormatUint(toBlock, 10)),
		nil,
	); err != nil {
		t.logger.Error(
			"cannot write last block to db",
			"err", err)
	}

	return logs
}

func (t *Tracker) subscribeNewHeads() error {
	// send subscription request
	request := ethSubscribeRequest{
		JsonRPC: "2.0",
		Method:  "eth_subscribe",
		Params:  []string{"newHeads"},
		Id:      1,
	}

	// 	prepare subscribe request
	bytes, err := json.Marshal(request)
	if err != nil {
		//	ErrMarshalJSON
		return err
	}

	// send req
	if err := t.wsConn.WriteMessage(
		websocket.TextMessage,
		bytes,
	); err != nil {
		//	ErrWriteWS
		return err
	}
	println("send subscribe request")

	// verify ok
	var res jsonrpc.SuccessResponse
	_, msg, err := t.wsConn.ReadMessage()
	if err != nil {
		//	ErrReadWS
		println("cannot read msg")

		return err
	}

	println(string(msg))

	if err := json.Unmarshal(msg, &res); err != nil {
		//	ErrUnmarshalJSON
		println("Unable to unmarshal WS response: %v", err)

		return err
	}

	// all good, ws is listening...
	return nil
}
