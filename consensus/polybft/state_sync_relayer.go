package polybft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"golang.org/x/sync/semaphore"
)

const (
	// maxParallelTxWorkers how many transactions can be sent in parallel
	maxParallelTxWorkers = 10
	// maxAttemptsToSend how many sending retries for one transaction
	maxAttemptsToSend = 6
)

var errFailedToExecuteStateSync = errors.New("failed to execute state sync")

// StateSyncRelayer is an interface that defines functions for state sync relayer
type StateSyncRelayer interface {
	PostBlock(req *PostBlockRequest) error
	Init() error
	Close()
}

// stateSyncProofRetriever is an interface that exposes function for retrieving state sync proof
type stateSyncProofRetriever interface {
	GetStateSyncProof(stateSyncID uint64) (types.Proof, error)
}

var _ StateSyncRelayer = (*dummyStakeSyncRelayer)(nil)

// dummyStakeSyncRelayer is a dummy implementation of a StateSyncRelayer
type dummyStakeSyncRelayer struct{}

func (d *dummyStakeSyncRelayer) PostBlock(req *PostBlockRequest) error { return nil }

func (d *dummyStakeSyncRelayer) Init() error { return nil }
func (d *dummyStakeSyncRelayer) Close()      {}

var _ StateSyncRelayer = (*stateSyncRelayerImpl)(nil)

// StateSyncRelayerStateData is used to keep state of state sync relayer
type StateSyncRelayerStateData struct {
	LastBlockNumber uint64 `json:"lastBlockNumber"`
}

// StateSyncRelayerEventData keeps information about an event
type StateSyncRelayerEventData struct {
	EventID    uint64 `json:"eventID"`
	CountTries uint64 `json:"countTries"`
	// block when state sync is sent
	BlockNumber uint64 `json:"blockNumber"`
	SentStatus  bool   `json:"sentStatus"`
}

type stateSyncRelayerImpl struct {
	txRelayer    txrelayer.TxRelayer
	key          ethgo.Key
	store        stateSyncProofRetriever
	state        *State
	eventsGetter *eventsGetter[*contractsapi.NewCommitmentEvent]
	logger       hclog.Logger
	blockchain   blockchainBackend

	sem      *semaphore.Weighted
	notifyCh chan struct{}
	closeCh  chan struct{}
}

func NewStateSyncRelayer(
	txRelayer txrelayer.TxRelayer,
	stateReceiverAddr types.Address,
	state *State,
	store stateSyncProofRetriever,
	blockchain blockchainBackend,
	key ethgo.Key,
	logger hclog.Logger,
) StateSyncRelayer {
	return &stateSyncRelayerImpl{
		txRelayer:  txRelayer,
		key:        key,
		store:      store,
		state:      state,
		sem:        semaphore.NewWeighted(10),
		closeCh:    make(chan struct{}),
		notifyCh:   make(chan struct{}, 1),
		blockchain: blockchain,
		eventsGetter: &eventsGetter[*contractsapi.NewCommitmentEvent]{
			blockchain: blockchain,
			isValidLogFn: func(l *types.Log) bool {
				return l.Address == stateReceiverAddr
			},
			parseEventFn: func(_ *types.Header, log *ethgo.Log) (*contractsapi.NewCommitmentEvent, bool, error) {
				var commitEvent contractsapi.NewCommitmentEvent

				doesMatch, err := commitEvent.ParseLog(log)

				return &commitEvent, doesMatch, err
			},
		},
		logger: logger,
	}
}

func (ssr *stateSyncRelayerImpl) Init() error {
	// start consumer
	go func() {
		for {
			select {
			case <-ssr.closeCh:
				return
			case <-ssr.notifyCh:
				ssr.processBatch()
			}
		}
	}()

	return nil
}

func (ssr *stateSyncRelayerImpl) Close() {
	close(ssr.closeCh)
}

func (ssr *stateSyncRelayerImpl) PostBlock(req *PostBlockRequest) error {
	state, err := ssr.state.StateSyncStore.getStateSyncRelayerStateData()
	if err != nil {
		return fmt.Errorf("state sync relayer get state failed: %w", err)
	}

	// create default state data if this is the first time
	if state == nil {
		state = &StateSyncRelayerStateData{LastBlockNumber: 0}
	}

	for state.LastBlockNumber+1 < req.FullBlock.Block.Header.Number {
		state.LastBlockNumber++

		events, err := ssr.eventsGetter.getEvents(state.LastBlockNumber)
		if err != nil {
			return fmt.Errorf("state sync relayer: %w", err)
		}

		if err = ssr.state.StateSyncStore.insertStateSyncRelayerStateData(state, getConvertedEvents(events)); err != nil {
			return fmt.Errorf("state sync relayer insert state failed: %w", err)
		}

		ssr.logger.Info("state sync relayer updated state", "block", state.LastBlockNumber)
	}

	events, err := ssr.eventsGetter.getEventsFromReceipts(req.FullBlock.Block.Header, req.FullBlock.Receipts)
	if err != nil {
		return err
	}

	state.LastBlockNumber = req.FullBlock.Block.Number()

	if err = ssr.state.StateSyncStore.insertStateSyncRelayerStateData(state, getConvertedEvents(events)); err != nil {
		return fmt.Errorf("state sync relayer insert state failed: %w", err)
	}

	ssr.logger.Info("state sync relayer updated state", "block", state.LastBlockNumber)

	select {
	case ssr.notifyCh <- struct{}{}:
	default:
	}

	return nil
}

func (ssr *stateSyncRelayerImpl) processBatch() {
	events, err := ssr.state.StateSyncStore.getAllAvailableEvents()
	if err != nil {
		ssr.logger.Error("error while reading available events", "err", err)

		return
	}

	for _, eventData := range events {
		if err := ssr.processEvent(eventData); err != nil {
			ssr.logger.Error("error while processing event", "err", err)
		}
	}
}

func (ssr *stateSyncRelayerImpl) processEvent(eventData *StateSyncRelayerEventData) error {
	eventData.CountTries++

	if eventData.CountTries >= maxAttemptsToSend {
		_ = ssr.state.StateSyncStore.removeStateSyncRelayerEvent(eventData.EventID)

		return fmt.Errorf("failed to send event too many times: %d", eventData.EventID)
	}

	proof, err := ssr.store.GetStateSyncProof(eventData.EventID)
	if err != nil {
		_ = ssr.state.StateSyncStore.updateStateSyncRelayerEvent(eventData)

		return fmt.Errorf("failed to query proof: %d, %w", eventData.EventID, err)
	}

	tx, err := ssr.createTx(proof)
	if err != nil {
		_ = ssr.state.StateSyncStore.updateStateSyncRelayerEvent(eventData)

		return fmt.Errorf("failed to create tx: %d, %w", eventData.EventID, err)
	}

	if err := ssr.sem.Acquire(context.Background(), 1); err != nil {
		// dont update state in this case!
		return fmt.Errorf("failed to acquire semaphore: %d, %w", eventData.EventID, err)
	}

	eventData.BlockNumber = ssr.blockchain.CurrentHeader().Number
	eventData.SentStatus = true

	if err := ssr.state.StateSyncStore.updateStateSyncRelayerEvent(eventData); err != nil {
		ssr.sem.Release(1)

		return fmt.Errorf("failed to move to sent: %d, %w", eventData.EventID, err)
	}

	go func() {
		defer ssr.sem.Release(1)

		if err := ssr.sendTx(tx); err != nil {
			ssr.logger.Error("failed to execute tx", "eventID", eventData.EventID, "err", err)

			eventData.SentStatus = false

			if err := ssr.state.StateSyncStore.updateStateSyncRelayerEvent(eventData); err != nil {
				ssr.logger.Error("failed to update store", "eventID", eventData.EventID, "err", err)
			}
		} else {
			ssr.logger.Info("state sync relayer has sent event", "eventID", eventData.EventID)

			if err := ssr.state.StateSyncStore.removeStateSyncRelayerEvent(eventData.EventID); err != nil {
				ssr.logger.Error("failed to update store", "eventID", eventData.EventID, "err", err)
			}
		}
	}()

	return nil
}

func (ssr *stateSyncRelayerImpl) createTx(proof types.Proof) (*ethgo.Transaction, error) {
	var sse *contractsapi.StateSync

	// since state sync event is a map in the jsonrpc response,
	// to not have custom logic of converting the map to state sync event
	// json encoding is used, since it manages to successfully unmarshal the
	// event from the marshaled map
	raw, err := json.Marshal(proof.Metadata["StateSync"])
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: %w", err)
	}

	if err = json.Unmarshal(raw, &sse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	input, err := (&contractsapi.ExecuteStateReceiverFn{
		Proof: proof.Data,
		Obj:   sse,
	}).EncodeAbi()
	if err != nil {
		return nil, err
	}

	// execute the state sync
	return &ethgo.Transaction{
		From:  ssr.key.Address(),
		To:    (*ethgo.Address)(&contracts.StateReceiverContract),
		Gas:   types.StateTransactionGasLimit,
		Input: input,
	}, nil
}

func (ssr *stateSyncRelayerImpl) sendTx(txn *ethgo.Transaction) error {
	receipt, err := ssr.txRelayer.SendTransaction(txn, ssr.key)
	if err != nil {
		return err
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		return runtime.ErrExecutionReverted
	}

	var stateSyncResult contractsapi.StateSyncResultEvent
	for _, log := range receipt.Logs {
		matches, err := stateSyncResult.ParseLog(log)
		if err != nil {
			return err
		}

		if !matches {
			continue
		}

		if !stateSyncResult.Status {
			return errFailedToExecuteStateSync
		}
	}

	return nil
}

func getStateSyncTxRelayer(rpcEndpoint string, logger hclog.Logger) (txrelayer.TxRelayer, error) {
	if rpcEndpoint == "" || strings.Contains(rpcEndpoint, "0.0.0.0") {
		_, port, err := net.SplitHostPort(rpcEndpoint)
		if err == nil {
			rpcEndpoint = fmt.Sprintf("http://%s:%s", "127.0.0.1", port)
		} else {
			rpcEndpoint = txrelayer.DefaultRPCAddress
		}
	}

	return txrelayer.NewTxRelayer(
		txrelayer.WithIPAddress(rpcEndpoint),
		txrelayer.WithWriter(logger.StandardWriter(&hclog.StandardLoggerOptions{})))
}

func getConvertedEvents(events []*contractsapi.NewCommitmentEvent) (newEvents []*StateSyncRelayerEventData) {
	for _, event := range events {
		for i := event.StartID.Uint64(); i <= event.EndID.Uint64(); i++ {
			newEvents = append(newEvents, &StateSyncRelayerEventData{EventID: i})
		}
	}

	return newEvents
}
