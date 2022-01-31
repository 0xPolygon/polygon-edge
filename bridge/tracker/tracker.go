package tracker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/0xPolygon/polygon-edge/jsonrpc"
	"github.com/0xPolygon/polygon-edge/types"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	rpc "github.com/umbracle/go-web3/jsonrpc"
)

const ethWebSocketURL = "wss://ropsten.infura.io/ws/v3/17eac086ff36442ebd43737400eb71ca"

type ethSubscribeRequest struct {
	JsonRPC string   `json:"jsonrpc"`
	Method  string   `json:"method"`
	Params  []string `json:"params"`
	Id      int      `json:"id"`
}

type ethSubscribeResponse struct {
	JSONRPC string `json:"jsonrpc"`
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

type tracker struct {
	logger hclog.Logger

	// cancel process

	// requiredConfirmations
	confirmations uint64

	// db storage (last-processed-block)

	// subscription channel
	headerCh chan *ethHeader

	// event channel
	eventCh chan []byte

	// websocket connection (newHeads subscription)
	wsConn *websocket.Conn

	// rpc client (eth_getLogs call)
	rpcClient *rpc.Client
}

func newTracker(confirmations uint64) (*tracker, error) {
	tracker := &tracker{
		logger:        logger.Named("event tracker"),
		confirmations: confirmations,
		headerCh:      make(chan *ethHeader),
		eventCh:       make(chan []byte),
	}

	// 	create db for last processed block num
	// 	or instantiate from existing file

	//	create logger

	return tracker, nil
}

func (t *tracker) Start() (<-chan []byte, error) {
	// create ws connection
	conn, _, err := websocket.DefaultDialer.Dial(
		ethWebSocketURL,
		nil,
	)
	if err != nil {
		println("cannot connect")
		return nil, err
	}

	t.wsConn = conn
	println("connected to ws endpoint")

	//	create http connection	TODO

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
		return nil, err
	}

	// send req
	if err := t.wsConn.WriteMessage(
		websocket.TextMessage,
		bytes,
	); err != nil {
		//	ErrWriteWS
		return nil, err
	}
	println("send subscribe request")

	// verify ok
	var res jsonrpc.SuccessResponse
	_, msg, err := t.wsConn.ReadMessage()
	if err != nil {
		//	ErrReadWS
		println("cannot read msg")

		return nil, err
	}

	println(string(msg))

	if err = json.Unmarshal(msg, &res); err != nil {
		//	ErrUnmarshalJSON
		println("Unable to unmarshal WS response: %v", err)

		return nil, err
	}
	// all good, ws is listening...
	println("subscribed - id:", res.ID)

	//	start the header process
	go func() {
		select {
		case header := <-t.headerCh:
			t.processHeader(header)
			//	case cancel
		}
	}()

	// listen for new heads...
	go func() {
		for {
			res := ethSubscribeResponse{}
			err := t.wsConn.ReadJSON(&res)
			if err != nil {
				//	ErrReadJSON
				println("listener: cant read message")
				continue
			}

			header := &ethHeader{}
			if err = json.Unmarshal(res.Params.Result, header); err != nil {
				//	ErrUnmarshalJSON
				fmt.Printf("cant parse result json - err: %v\n", err)
				continue
			}

			println(header.Difficulty)

			//	process header for events
			t.headerCh <- header
		}
	}()

	return t.eventCh, nil
}

func (t *tracker) Stop() {
	//	disconnect

	//	stop subscription

	//	stop the header process

}

func (t *tracker) processHeader(header *ethHeader) {
	//	determine range of blocks to query
	fromBlock, toBlock := t.calculateRange(header)

	//	create log filter

	//	return matching event logs
	logs := t.queryEvents(fromBlock, toBlock /* log filter */)

	//	send events
	t.notify(logs...)
	//	construct log filter from given abis
	//	and issue eth_getLogs request to the rootchain

	//	for each event matches against an abi
	//	send the log (bytes) to the tracker's channel
}

func (t *tracker) calculateRange(header *ethHeader) (from, to uint64) {
	//	extract block number from header field

	//	fetch the last processed from db

	//	determine new range to query (from == too, possible)

	//	update db (?)

	return 0, 0
}

func (t *tracker) queryEvents(fromBlock, toBlock uint64) {
	//	construct rpc request using
	//	fromBlock, toBlock and log filter

	//	send the request

	//	receive the response

	//	unmarshal into type Log

	//	return logs
}

func (t *tracker) notify(logs ...*Log) {

}

func (t *tracker) subscribeNewHeads(ctx context.Context) {

}
