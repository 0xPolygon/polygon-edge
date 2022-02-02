package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"

	"github.com/0xPolygon/polygon-edge/jsonrpc"
	"github.com/0xPolygon/polygon-edge/types"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/umbracle/go-web3"
	rpc "github.com/umbracle/go-web3/jsonrpc"
)

const (
	ethWS              = "wss://ropsten.infura.io/ws/v3/17eac086ff36442ebd43737400eb71ca"
	ethHTTP            = "https://mainnet.infura.io/v3/58f8f6612b494cac85e2c8ab2ce11ed1"
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

type ethHeader struct {
	Difficulty   string        `json:"difficulty"`
	ExtraData    []byte        `json:"extraData"`
	GasLimit     string        `json:"gasLimit"`
	GasUsed      string        `json:"gasUsed"`
	LogsBloom    types.Bloom   `json:"logsBloom"`
	Miner        types.Address `json:"miner"`
	Nonce        string        `json:"nonce"`
	Number       string        `json:"number"`
	ParentHash   types.Hash    `json:"parentHash"`
	ReceiptsRoot types.Hash    `json:"receiptsRoot"`
	Sha3Uncles   types.Hash    `json:"sha3Uncles"`
	StateRoot    types.Hash    `json:"stateRoot"`
	Timestamp    string        `json:"timestamp"`
	TxRoot       types.Hash    `json:"transactionsRoot"`
	MixHash      types.Hash    `json:"mixHash"`
	Hash         types.Hash    `json:"hash"`
}

type Tracker struct {
	logger hclog.Logger

	// cancel funcs
	cancelSubscription, cancelHeaderProcess context.CancelFunc

	// requiredConfirmations
	confirmations uint64

	// subscription channel
	headerCh chan *ethHeader

	// event channel
	eventCh chan []byte

	// websocket connection (newHeads subscription)
	wsConn *websocket.Conn

	// rpc client (eth_getLogs call)
	rpcClient *rpc.Client

	//	db (last processed block)
	db *leveldb.DB
}

func NewEventTracker(logger hclog.Logger, confirmations uint64) (*Tracker, error) {
	tracker := &Tracker{
		logger:        logger.Named("event Tracker"),
		confirmations: confirmations,
		headerCh:      make(chan *ethHeader),
		eventCh:       make(chan []byte),
	}

	// 	create db for last processed block num
	// 	or instantiate from existing file
	if cwd, err := os.Getwd(); err != nil {
		println("cannot fetch cwd")
		return nil, err
	} else {
		if db, err := leveldb.OpenFile(
			cwd+"/last_block",
			nil,
		); err != nil {
			return nil, err
		} else {
			tracker.db = db
		}
	}

	return tracker, nil
}

func (t *Tracker) Start() (<-chan []byte, error) {
	if err := t.connect(); err != nil {
		//	log
		t.logger.Error("could not connect", "err", err)
		return nil, err
	}

	if err := t.subscribeNewHeads(); err != nil {
		t.logger.Error("could not subscribe", "err", err)

		return nil, err
	}

	//	create cancellable contexts
	ctxSubscription, cancelSub := context.WithCancel(context.Background())
	t.cancelSubscription = cancelSub

	ctxHeaderProcess, cancelHeader := context.WithCancel(context.Background())
	t.cancelHeaderProcess = cancelHeader

	//	start the header processing early
	go t.startHeaderProcess(ctxSubscription)

	//	start processing new headers from subscription
	go t.startSubscription(ctxHeaderProcess)

	return t.eventCh, nil
}

func (t *Tracker) Stop() error {
	//	stop subscription
	t.cancelSubscription()

	// 	stop processing headers
	t.cancelHeaderProcess()

	//	disconnect
	if err := t.rpcClient.Close(); err != nil {
		return err
	}

	if err := t.wsConn.Close(); err != nil {
		return err
	}

	return nil
}

func (t *Tracker) connect() error {
	// create ws connection
	conn, _, err := websocket.DefaultDialer.Dial(
		ethWS,
		nil,
	)
	if err != nil {
		//	ErrConnectWS
		println("cannot connect")
		return err
	}

	t.wsConn = conn
	println("connected to ws endpoint")

	//	create http connection
	rpcClient, err := rpc.NewClient(ethHTTP)
	if err != nil {
		//	ErrConnectHTTP
		println("cannot connect")
		return err
	}

	t.rpcClient = rpcClient
	println("connected to http endpoint")

	return nil
}

func (t *Tracker) startSubscription(ctx context.Context) {
	for {
		//	check if subscription is closed
		select {
		case <-ctx.Done():
			return
		default:
		}

		res := ethSubscribeResponse{}
		err := t.wsConn.ReadJSON(&res)
		if err != nil {
			//	ErrReadJSON
			println("listener: cant read message")
			return
		}

		header := &ethHeader{}
		if err = json.Unmarshal(res.Params.Result, header); err != nil {
			//	ErrUnmarshalJSON
			fmt.Printf("cant parse result json - err: %v\n", err)
			continue
		}

		println(header.Number)

		//	process header for events
		t.headerCh <- header
	}
}

func (t *Tracker) startHeaderProcess(ctx context.Context) {
	for {
		select {
		case header := <-t.headerCh:
			t.processHeader(header)
		case <-ctx.Done():
			return
		}
	}
}

func (t *Tracker) processHeader(header *ethHeader) {
	//	determine range of blocks to query
	fromBlock, toBlock := t.calculateRange(header)

	//	fetch logs
	logs := t.queryEvents(fromBlock, toBlock)

	if len(logs) == 0 {
		println("No events")
	}

	//	match and dispatch
	for _, log := range logs {
		if NewRegistrationEvent.Match(log) {
			//	match
			continue
		}
		if RegistrationUpdatedEvent.Match(log) {
			//	match
			continue
		}
		if StateSyncedEvent.Match(log) {
			//	match
			continue
		}
		if PoCEvent.Match(log) {
			t.logger.Info("PoC event", "contract address", log.Address.String())
			continue
		}
	}
}

func (t *Tracker) calculateRange(header *ethHeader) (from, to uint64) {
	//	extract block number from header field
	latestHeight, err := types.ParseUint256orHex(&header.Number)
	if err != nil {
		println("cannot parse block number from hex")
		//	cannot parse
		return
	}

	println("latest block number", latestHeight.Uint64())

	confirmationBlocks := big.NewInt(0).SetUint64(t.confirmations)
	if latestHeight.Cmp(confirmationBlocks) < 0 {
		//	block height less than required
		println("not enough confirmations")

		return
	}

	//	right bound (latest - confirmations)
	toBlock := big.NewInt(0).Sub(latestHeight, confirmationBlocks)

	//	left bound
	var fromBlock *big.Int
	if exists, _ := t.db.Has(
		[]byte(lastProcessedBlock),
		nil,
	); exists {
		bytesLastBlock, err := t.db.Get(
			[]byte(lastProcessedBlock),
			nil,
		)
		if err != nil {
			//	log (cannot get db)
			println("cannot read db (last processed block)")
			return
		}

		lastBlock, ok := big.NewInt(0).SetString(string(bytesLastBlock), 10)
		if ok {
			//	increment
			fromBlock = lastBlock.Add(
				lastBlock,
				big.NewInt(1),
			)
		} else {
			//	cannot parse
			println("cannot parse db value")
			return
		}

	} else {
		println("db empty")
		fromBlock = toBlock
	}

	println("calculated range", fromBlock.Uint64(), toBlock.Uint64())

	switch fromBlock.Cmp(toBlock) {
	case -1:
		//	from < to
		from, to = fromBlock.Uint64(), toBlock.Uint64()
	case 0, 1:
		//	from == to, from > to
		from, to = fromBlock.Uint64(), fromBlock.Uint64()
	}

	println("calculated range", from, to)

	//	update db (?)
	if err := t.db.Put(
		[]byte(lastProcessedBlock),
		[]byte(toBlock.String()),
		nil,
	); err != nil {
		println("could not store last block processed")
	}

	println("stored in db", toBlock.Uint64())

	return
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
		println("eth_getLogs failed", err)
		return nil
	}

	return logs
}

//
//func (t *Tracker) notify(logs ...*Log) {
//
//}

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
