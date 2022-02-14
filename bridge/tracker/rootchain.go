package tracker

import (
	"encoding/json"
	"fmt"

	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/abi"
	client "github.com/umbracle/go-web3/jsonrpc"
)

const (
	//	Ropsten testnet
	//rootchainWS   = "wss://ropsten.infura.io/ws/v3/17eac086ff36442ebd43737400eb71ca"

	//	Polygon Edge
	rootchainWS = "ws://127.0.0.1:10002/ws"
)

const (
	//	Smart contract addresses
	PoCSC          = "19DC3Af00E7f7502a2A40B7e0FeA194A86CeAA0c"
	AnotherEventSC = "69ceed5Ff0FA5106F4Df7299C8812377394A9388"
	ThirdEventSC   = "b22a1Cd34d39D46bB2f077bd6295c850702D8e81"
)

var (
	//	ABI events (defined in the above smart contracts)

	/*	StateSender.sol	*/
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

	/*	PoC contract events	*/
	PoCEvent      = abi.MustNewEvent(`event MyEvent(address indexed sender)`)
	topicPoCEvent = PoCEvent.ID()

	AnotherEvent      = abi.MustNewEvent(`event AnotherEvent(address indexed sender)`)
	topicAnotherEvent = AnotherEvent.ID()

	ThirdEvent = abi.MustNewEvent(`event ThirdEvent(address indexed sender)`)
)

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
