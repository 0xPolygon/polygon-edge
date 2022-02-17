package tracker

import (
	"encoding/json"
	"fmt"
	"github.com/0xPolygon/polygon-edge/blockchain/storage"
	"github.com/0xPolygon/polygon-edge/blockchain/storage/leveldb"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/abi"
	client "github.com/umbracle/go-web3/jsonrpc"
	"math/big"
	"os"
	"path/filepath"
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
func initRootchainDB(logger hclog.Logger, dbPath string) (storage.Storage, error) {
	if dbPath == "" {
		dbPath, _ = os.Getwd()
		dbPath = filepath.Join(dbPath, "last-processed-block")
	}

	db, err := leveldb.NewLevelDBStorage(dbPath, logger)
	if err != nil {
		return nil, err
	}

	return db, nil
}

/*	Rootchain sub object	*/

type cancelSubCallback func() error

//	rootchain subscription object
type subscription struct {
	newHeadsCh chan *ethHeader
	errorCh    chan error
	cancel     cancelSubCallback
}

//	newHead returns the subscription's channel for new head events.
func (s *subscription) newHead() <-chan *ethHeader {
	return s.newHeadsCh
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
func (s *subscription) handleWSResponse(response []byte) {
	//	parse ws response
	var header *ethHeader
	if err := json.Unmarshal(response, header); err != nil {
		s.errorCh <- fmt.Errorf("unable to parse header - err: %w", err)
		return
	}

	//	emit header
	s.newHeadsCh <- header
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
func (c *rootchainClient) subscribeNewHeads() (subscription, error) {
	//	create sub object
	sub := subscription{
		newHeadsCh: make(chan *ethHeader, 1),
		errorCh:    make(chan error, 1),
	}

	//	subscribe to rootchain
	cancelSub, err := c.impl.Subscribe("newHeads", sub.handleWSResponse)
	if err != nil {
		return sub, err
	}

	//	save cancel callback
	sub.cancel = cancelSub

	return sub, nil
}

//	getLogs returns all log events from the rootchain matching the filter's criteria.
func (c *rootchainClient) getLogs(filter *web3.LogFilter) ([]*web3.Log, error) {
	return c.impl.Eth().GetLogs(filter)
}
