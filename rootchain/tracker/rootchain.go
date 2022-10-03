package tracker

import (
	"encoding/json"
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	client "github.com/umbracle/ethgo/jsonrpc"
)

// rootchainClient is a wrapper object for the ethgo client.
type rootchainClient struct {
	impl *client.Client
}

// newRootchainClient returns a new client connected to the rootchain.
func newRootchainClient(addr string) (*rootchainClient, error) {
	impl, err := client.NewClient(addr)
	if err != nil {
		return nil, err
	}

	return &rootchainClient{impl: impl}, nil
}

// subscribeNewHeads returns a subscription for new header events.
func (c *rootchainClient) subscribeNewHeads() (subscription, error) {
	// create sub object
	sub := subscription{
		newHeadsCh: make(chan *types.Header, 1),
		errorCh:    make(chan error, 1),
	}

	// subscribe to rootchain
	cancelSub, err := c.impl.Subscribe("newHeads", sub.handleWSResponse)
	if err != nil {
		return sub, err
	}

	// save cancel callback
	sub.cancel = cancelSub

	return sub, nil
}

// close closes the client's connection to the rootchain.
func (c *rootchainClient) close() error {
	return c.impl.Close()
}

// getLogs returns all log events from the rootchain matching the filter's criteria.
func (c *rootchainClient) getLogs(filter *ethgo.LogFilter) ([]*ethgo.Log, error) {
	return c.impl.Eth().GetLogs(filter)
}

type cancelSubCallback func() error

// subscription is rootchain subscription object
type subscription struct {
	newHeadsCh chan *types.Header
	errorCh    chan error
	cancel     cancelSubCallback
}

// newHead returns the subscription's channel for new head events.
func (s *subscription) newHead() <-chan *types.Header {
	return s.newHeadsCh
}

// unsubscribe cancels the subscription.
func (s *subscription) unsubscribe() error {
	return s.cancel()
}

// handleWSResponse parses the json response
// received by the websocket into a header struct.
func (s *subscription) handleWSResponse(response []byte) {
	// parse ws response
	header := &types.Header{}
	if err := json.Unmarshal(response, header); err != nil {
		s.errorCh <- fmt.Errorf("unable to parse header - err: %w", err)

		return
	}

	// emit header
	s.newHeadsCh <- header
}

// err returns the subscription's error channel.
func (s *subscription) err() <-chan error {
	return s.errorCh
}

// contractABI is used to create query filter
// that matches events defined in a smart contract.
type contractABI struct {
	// address of smart contract
	address ethgo.Address

	// signatures of events defined in smart contract
	event *abi.Event
}

// eventIDs returns all the event signatures (IDs)
func (c *contractABI) eventIDs() (IDs []*ethgo.Hash) {
	id := c.event.ID()
	IDs = append(IDs, &id)

	return
}
