package tracker

import (
	"context"
	"encoding/json"
	"errors"
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
	//rootchainWS          = "wss://ropsten.infura.io/ws/v3/17eac086ff36442ebd43737400eb71ca"
	//rootchainHTTP        = "https://ropsten.infura.io/v3/58f8f6612b494cac85e2c8ab2ce11ed1"

	//	edge
	rootchainWS   = "ws://127.0.0.1:10002/ws"
	rootchainHTTP = "http://127.0.0.1:10002"

	lastProcessedBlock = "last-processed-block"
	//stateSenderAddress = "74FbD47E7390E345982A3b7e413D35332945C10C"
	//stateSenderAddress = "1A2dB8920ed2d8E4D14b3091DA0c4febEbdd7BCc"
	PoCSC          = "19DC3Af00E7f7502a2A40B7e0FeA194A86CeAA0c"
	AnotherEventSC = "69ceed5Ff0FA5106F4Df7299C8812377394A9388"
)

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

	//	db for last processed block number
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
//func (t *Tracker) getEventChannel() <-chan []byte {
//	return t.eventCh
//}

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

//	connect connects the tracker to rootchain's http and ws endpoints.
func (t *Tracker) connect() error {
	// create ws connection
	conn, _, err := websocket.DefaultDialer.Dial(
		rootchainWS,
		nil)
	if err != nil {
		t.logger.Error(
			"ws: cannot connect",
			"err", err)

		return err
	}

	t.wsConn = conn
	t.logger.Debug("connected to ws endpoint")

	//	create http connection
	rpcClient, err := rpc.NewClient(rootchainHTTP)
	if err != nil {
		t.logger.Error("http: cannot connect")

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

		default:
			res := ethSubscribeResponse{}
			//	read the subscription message
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
			t.logger.Debug("stopping header process")

			return
		}
	}
}

//	processHeader determines the range of block to query
//	based on the blockNumber of the header and issues
//	eth_getLogs call. If an event of interest was emitted,
//	it is sent to eventCh.
func (t *Tracker) processHeader(header *ethHeader) {
	t.logger.Info("processing header", "num", header.Number)

	//	determine range of blocks to query
	fromBlock, toBlock := t.calculateRange(header)

	//	fetch logs
	logs := t.queryEvents(fromBlock, toBlock)

	//	notify each matched log event
	t.matchAndDispatch(logs)
}

func (t *Tracker) matchAndDispatch(logs []*web3.Log) {
	if len(logs) == 0 {
		t.logger.Debug("no events")
		return
	}

	//	TODO: refactor once PoC is reached
	//	Events will be matched by server,
	//	when they are included as topics
	//	in queryFilter()

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

		if AnotherEvent.Match(log) {
			t.logger.Info("AnotherEvent", "contract address", log.Address.String())

			if err := t.notify(log); err != nil {
				t.logger.Error(
					"cannot marshal log",
					"err", err)
			}

			continue
		}
	}
}

func (t *Tracker) calculateRange(header *ethHeader) (from, to uint64) {
	//	extract block number from header field
	latestHeight := big.NewInt(0).SetUint64(header.Number)
	//latestHeight, err := types.ParseUint256orHex(&header.Number)	TODO (ethHeader)

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
		//	increment last block number
		fromBlock = lastBlock.Add(
			lastBlock,
			big.NewInt(1))
	}

	if toBlock.Cmp(fromBlock) < 0 {
		fromBlock = toBlock
	}

	return fromBlock.Uint64(), toBlock.Uint64()
}

//	loadLastBlock returns the block number of the last
// 	block processed by the tracker (if available).
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
			web3.HexToAddress(PoCSC),
			web3.HexToAddress(AnotherEventSC),
		},
	}

	queryFilter.SetFromUint64(fromBlock)
	queryFilter.SetToUint64(toBlock)

	t.logger.Debug("eth_getLogs",
		"from", fromBlock,
		"to", toBlock,
	)

	logs, err := t.rpcClient.Eth().GetLogs(queryFilter)
	if err != nil {
		println("eth_getLogs failed", err.Error())
		t.logger.Error("eth_getLogs failed", "err", err)

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
	select {
	case t.eventCh <- bLog:
	default:
	}

	return nil
}

func (t *Tracker) subscribeNewHeads() error {
	// 	prepare subscribe request
	request := ethSubscribeRequest{
		JSONRPC: "2.0",
		Method:  "eth_subscribe",
		Params:  []string{"newHeads"},
		ID:      1,
	}

	bytes, err := json.Marshal(request)
	if err != nil {
		return errors.New("failed to marshal subscribe request")
	}

	// subscribe
	if err := t.wsConn.WriteMessage(
		websocket.TextMessage,
		bytes,
	); err != nil {
		return errors.New("failed to write ws message")
	}

	// receive subscription response
	var res jsonrpc.SuccessResponse

	_, msg, err := t.wsConn.ReadMessage()
	if err != nil {
		return errors.New("failed to read ws message")
	}

	if err := json.Unmarshal(msg, &res); err != nil {
		return errors.New("failed to unmarshal response")
	}

	return nil
}

/* Structures used for message parsing (json) */

type ethSubscribeRequest struct {
	JSONRPC string   `json:"jsonrpc"`
	Method  string   `json:"method"`
	Params  []string `json:"params"`
	ID      int      `json:"id"`
}

type ethSubscribeResponse struct {
	JSONRPC string `json:"jsonrpc"`
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
