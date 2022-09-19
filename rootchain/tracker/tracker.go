package tracker

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/0xPolygon/polygon-edge/rootchain"
	"github.com/0xPolygon/polygon-edge/rootchain/payload"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

var (
	ErrInvalidID            = errors.New("id isn't in event or wrong type")
	ErrInvalidValidatorsMap = errors.New("validators map is not in event or wrong type")
)

type Validator struct {
	EcdsaAddress types.Address
	BlsPublicKey []byte
}

// cancellable context for tracker's listening mechanism
type contextSubscription struct {
	context context.Context
	cancel  context.CancelFunc
}

// done returns the contexts Done channel
func (c *contextSubscription) done() <-chan struct{} {
	return c.context.Done()
}

// EventTracker represents an event listener that notifies
// for each event emitted by a smart contract on the rootchain.
// Events are collected from blocks at the desired depth (confirmations).
type EventTracker struct {
	logger hclog.Logger

	// required block confirmations
	confirmations uint64

	// event channel
	eventCh chan rootchain.Event

	// cancel subscription
	ctxSubscription contextSubscription

	// rootchain subscription object
	sub subscription

	// rootchain client
	client *rootchainClient

	// events that tracker will listen for
	contract *contractABI

	// from which block to fetch events
	fromBlock uint64

	// payloadType type of the payload
	payloadType rootchain.PayloadType
}

// NewEventTracker returns a new tracker object.
func NewEventTracker(
	logger hclog.Logger,
	eventConfig *rootchain.ConfigEvent,
	rootchainWS string,
) (*EventTracker, error) {
	if eventConfig == nil {
		return nil, errors.New("no event config provided")
	}

	// create tracker
	tracker := &EventTracker{
		logger:        logger.Named("event_tracker"),
		confirmations: eventConfig.BlockConfirmations,
		eventCh:       make(chan rootchain.Event, 1000),
		payloadType:   eventConfig.PayloadType,
	}

	// load abi events
	tracker.contract = loadABI(eventConfig.LocalAddress, eventConfig.EventABI)

	var err error

	// create rootchain client
	if tracker.client, err = newRootchainClient(rootchainWS); err != nil {
		logger.Error("cannot connect to rootchain", "err", err)

		return nil, err
	}

	return tracker, nil
}

// Start initializes context for the tracking mechanism
// and starts listening for events on the rootchain.
func (t *EventTracker) Start(fromBlock uint64) error {
	// subscribe for new headers
	if err := t.subscribeToRootchain(); err != nil {
		t.logger.Error("cannot subscribe to rootchain", "err", err)

		return err
	}

	// set from block
	t.setFromBlock(fromBlock)

	// start processing new header events
	go t.startEventTracking()

	return nil
}

// Stop stops the tracker's listening mechanism.
func (t *EventTracker) Stop() error {
	// stop subscription
	t.ctxSubscription.cancel()

	// close rootchain client
	if err := t.client.close(); err != nil {
		t.logger.Error("cannot close rootchain client", "err", err)

		return err
	}

	return nil
}

// getEventChannel returns the tracker's event channel.
func (t *EventTracker) GetEventChannel() <-chan rootchain.Event {
	return t.eventCh
}

// subscribeToRootchain subscribes the tracker for new
// header events on the rootchain.
func (t *EventTracker) subscribeToRootchain() error {
	// create cancellable context for tracker
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.ctxSubscription = contextSubscription{
		context: ctx,
		cancel:  cancelFunc,
	}

	// create subscription for new header events
	var err error
	if t.sub, err = t.client.subscribeNewHeads(); err != nil {
		return err
	}

	return nil
}

// startEventTracking handles the subscription object (provided by the rootchain client).
func (t *EventTracker) startEventTracking() {
	for {
		select {
		// process new header
		case header := <-t.sub.newHead():
			t.trackHeader(header)

		// handle sub error
		case err := <-t.sub.err():
			t.ctxSubscription.cancel()
			t.logger.Error("subscription cancelled", err)

			return

		// stop tracker's sub
		case <-t.ctxSubscription.done():
			if err := t.sub.unsubscribe(); err != nil {
				t.logger.Error("cannot unsubscribe", "err", err)

				return
			}

			t.logger.Debug("subscription stopped")

			return
		}
	}
}

// setFromBlock sets a from block attribute
func (t *EventTracker) setFromBlock(fromBlock uint64) {
	t.fromBlock = fromBlock
}

// trackHeader determines the range of block to query
// based on the blockNumber of the header and issues an appropriate
// eth_getLogs call. If an event of interest was emitted,
// it is sent to eventCh.
func (t *EventTracker) trackHeader(header *types.Header) {
	// determine range of blocks to query
	fromBlock, toBlock := t.calculateRange(header)
	if toBlock == 0 && fromBlock == 0 {
		// we are returning here because the calculated
		// range was already queried or the chain is not
		// at the desired depth.
		return
	}

	t.logger.Info("querying events within block range", "from", fromBlock, "to", toBlock)

	// fetch logs
	logs := t.queryEvents(fromBlock, toBlock)
	t.logger.Info("matched events", "num", len(logs))

	// notify each matched log event
	t.notify(logs...)

	// update left bound
	t.setFromBlock(toBlock + 1)
}

// calculateRange determines the next range of blocks
// to query for events.
func (t *EventTracker) calculateRange(header *types.Header) (from, to uint64) {
	// check if block number is at required depth
	if header.Number < t.confirmations {
		// block height less than required
		t.logger.Debug(
			"not enough confirmations",
			"current", header.Number,
			"required", t.confirmations)

		return 0, 0
	}

	// right bound
	toBlock := header.Number - t.confirmations

	// left bound
	fromBlock := t.fromBlock

	// If tracker started with max uint64 only query events
	// from the latest valid block
	if t.fromBlock == math.MaxUint64 {
		fromBlock = toBlock
	}

	return fromBlock, toBlock
}

// queryEvents collects all events on the rootchain that occurred
// between blocks fromBlock and toBlock (inclusive).
func (t *EventTracker) queryEvents(fromBlock, toBlock uint64) []*ethgo.Log {
	// create the query filter
	queryFilter := setupQueryFilter(fromBlock, toBlock, t.contract)

	// call eth_getLogs
	logs, err := t.client.getLogs(queryFilter)
	if err != nil {
		t.logger.Error("eth_getLogs failed", "err", err)

		return nil
	}

	return logs
}

// notify sends the given logs to the event channel.
func (t *EventTracker) notify(logs ...*ethgo.Log) {
	for _, log := range logs {
		event, err := t.encodeEventFromLog(log)
		if err != nil {
			t.logger.Error(err.Error())
		}

		// notify [BLOCKING]
		t.eventCh <- event
	}
}

// encodeEventFromLog encodes event from log
func (t *EventTracker) encodeEventFromLog(log *ethgo.Log) (rootchain.Event, error) {
	eventData, err := t.contract.event.ParseLog(log)
	if err != nil {
		return rootchain.Event{}, errors.New(fmt.Sprint("cannot parse event log", "err", err))
	}

	switch t.payloadType {
	case rootchain.ValidatorSetPayloadType:
		return t.encodeValidatorSetPayloadEvent(eventData, log.BlockNumber)
	}

	return rootchain.Event{}, errors.New("cannot parse event, no payloadType defined")
}

// encodeEventFromLog encodes event for specific payload
func (t *EventTracker) encodeValidatorSetPayloadEvent(
	eventData map[string]interface{},
	blockNumber uint64,
) (rootchain.Event, error) {
	var (
		index *big.Int
		ok    bool

		validators []map[string]interface{}
	)

	index, ok = eventData["index"].(*big.Int)
	if !ok {
		return rootchain.Event{}, errors.New(fmt.Sprint("failed to parse StateSyncEvent: %w", "err", ErrInvalidID))
	}

	validators, ok = eventData["Validator"].([]map[string]interface{})
	if !ok {
		return rootchain.Event{}, errors.New(fmt.Sprint("failed to parse StateSyncEvent: %w", "err", ErrInvalidValidatorsMap))
	}

	validatorSetInfo := make([]payload.ValidatorSetInfo, len(validators))

	for index, validatorInfo := range validators {
		blsKey, ok := validatorInfo["blsPublicKey"].([]byte)
		if !ok {
			return rootchain.Event{}, errors.New("failed to parse blsPublicKey from validator info")
		}

		ecdsaAddress, ok := validatorInfo["ecdsaAddress"].(ethgo.Address)
		if !ok {
			return rootchain.Event{}, errors.New("failed to parse ecdsaAddress from validator info")
		}

		newValidatorSetInfo := payload.ValidatorSetInfo{
			Address:      ecdsaAddress.Bytes(),
			BLSPublicKey: blsKey}

		validatorSetInfo[index] = newValidatorSetInfo
	}

	validatorSetPayload := payload.NewValidatorSetPayload(validatorSetInfo)

	event := rootchain.Event{
		Index:       index.Uint64(),
		BlockNumber: blockNumber,
		Payload:     validatorSetPayload,
	}

	return event, nil
}

// loadABIs parses contracts from raw map.
func loadABI(contractAddress string, EventABI string) (contract *contractABI) {
	// set smart contract address
	contract = &contractABI{
		address: ethgo.HexToAddress(contractAddress),
	}

	// set event (defined in contract)
	contract.event = abi.MustNewEvent(EventABI)

	return
}

// setupQueryFilter creates a log filter for the desired
// block range. Filter matches events defined in rootchain.go.
func setupQueryFilter(from, to uint64, contract *contractABI) *ethgo.LogFilter {
	queryFilter := &ethgo.LogFilter{}

	// set range of blocks to query
	queryFilter.SetFromUint64(from)
	queryFilter.SetToUint64(to)

	// set contract addresses and topics
	queryFilter.Address = append(queryFilter.Address, contract.address)

	// topics from all contracts must be in Topics[0]
	queryFilter.Topics = append(queryFilter.Topics, contract.eventIDs())

	return queryFilter
}
