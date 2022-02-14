package tracker

import (
	"encoding/json"
	"fmt"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/abi"
	client "github.com/umbracle/go-web3/jsonrpc"
	"math/big"
	"os"
)

const (
	//	Ropsten testnet
	//rootchainWS   = "wss://ropsten.infura.io/ws/v3/17eac086ff36442ebd43737400eb71ca"

	//	Polygon Edge
	rootchainWS = "ws://127.0.0.1:10002/ws"

	//	Smart contract addresses
	StateSender = ""
)

var (
	//	db key for saving tracker's progress (chain height)
	lastQueriedBlockNumber = []byte("last-block-num")

	/*	ABI events (defined in the above smart contracts) */

	//	StateSender
	NewRegistrationEvent = abi.MustNewEvent(`event NewRegistration(
	address indexed user,
	address indexed sender,
    address indexed receiver)`,
	)

	RegistrationUpdatedEvent = abi.MustNewEvent(`event RegistrationUpdated(
	address indexed user,
	address indexed sender,
	address indexed receiver)`,
	)

	StateSyncedEvent = abi.MustNewEvent(`event StateSynced(
	uint256 indexed id,
	address indexed contractAddress,
	bytes data)`,
	)
)

//	setupQueryFilter creates a log filter for the desired
//	block range. Filter matches events defined in rootchain.go.
func setupQueryFilter(from, to *big.Int) *web3.LogFilter {
	queryFilter := &web3.LogFilter{}

	//	set range of blocks to query
	queryFilter.SetFromUint64(from.Uint64())
	queryFilter.SetToUint64(to.Uint64())

	/*	SC addresses and event topics are set here */

	queryFilter.Address = []web3.Address{
		//	set smart contract addresses
	}

	queryFilter.Topics = [][]*web3.Hash{
		{
			//	set event topics
		},
	}

	return queryFilter
}

/* Header types parsed by the client */

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

/* 	Rootchain storage (last processed block number) */

//	initRootchainDB creates a new database (or loads existing)
//	for storing the last processed block's number by the tracker.
func initRootchainDB() (*leveldb.DB, error) {
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

/*	Rootchain subscription object	*/

type cancelSubCallback func() error

//	rootchain subscription object
type subscription struct {
	errorCh chan error
	cancel  cancelSubCallback
}

//	unsubscribe cancels the subscription.
func (s *subscription) unsubscribe() error {
	return s.cancel()
}

//	err	returns the subscription's error channel.
func (s *subscription) err() <-chan error {
	return s.errorCh
}

//	handleWSResponse parses the json response
//	received by the websocket into a header struct.
func (s *subscription) handleWSResponse(response []byte) (*ethHeader, error) {
	header := &ethHeader{}
	if err := json.Unmarshal(response, header); err != nil {
		return nil, err
	}

	return header, nil
}

/*	Rootchain client */

//	rootchainClient is a wrapper object for the web3 client.
type rootchainClient struct {
	impl *client.Client
}

//	newRootchainClient returns a new client connected to the rootchain.
func newRootchainClient(addr string) (*rootchainClient, error) {
	impl, err := client.NewClient(addr)
	if err != nil {
		return nil, err
	}

	return &rootchainClient{impl: impl}, nil
}

//	close closes the client's connection to the rootchain.
func (c *rootchainClient) close() error {
	return c.impl.Close()
}

//	subscribeNewHeads returns a subscription for new header events.
//	Each header received is sent to headerCh for further processing.
func (c *rootchainClient) subscribeNewHeads(headerCh chan<- *ethHeader) (subscription, error) {
	sub := subscription{errorCh: make(chan error, 1)}
	cancelSub, err := c.impl.Subscribe("newHeads", func(b []byte) {
		//	parse ws response
		header, err := sub.handleWSResponse(b)
		if err != nil {
			//	send error to subscription object
			err := fmt.Errorf("unable to parse header - err: %w", err)
			sub.errorCh <- err

			return
		}

		//	send header for processing
		headerCh <- header
	})

	if err != nil {
		return sub, err
	}

	sub.cancel = cancelSub

	return sub, nil
}

//	getLogs returns all log events from the rootchain matching the filter's criteria.
func (c *rootchainClient) getLogs(filter *web3.LogFilter) ([]*web3.Log, error) {
	return c.impl.Eth().GetLogs(filter)
}
