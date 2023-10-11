package polybft

import (
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
)

const (
	// maxAttemptsToSend specifies how many sending retries for one transaction
	maxAttemptsToSend = 6
)

var (
	errFailedToExecuteStateSync  = errors.New("failed to execute state sync")
	errAlreadyProcessedStateSync = errors.New("StateReceiver: STATE_SYNC_IS_PROCESSED")
)

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

	events, err := ssr.eventsGetter.getFromBlocks(state.LastBlockNumber, req.FullBlock)
	if err != nil {
		return fmt.Errorf("state sync relayer: %w", err)
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
	ssr.logger.Info("state sync relayer processing event", "eventID", eventData.EventID)

	eventData.CountTries++

	if eventData.CountTries > maxAttemptsToSend {
		_ = ssr.state.StateSyncStore.removeStateSyncRelayerEvent(eventData.EventID)

		return fmt.Errorf("failed to send event too many times: %d", eventData.EventID)
	}

	tx, err := ssr.createTx(eventData.EventID)
	if err != nil {
		_ = ssr.state.StateSyncStore.updateStateSyncRelayerEvent(eventData)

		return fmt.Errorf("failed to create tx: %d, %w", eventData.EventID, err)
	}

	eventData.BlockNumber = ssr.blockchain.CurrentHeader().Number
	eventData.SentStatus = true

	if err := ssr.state.StateSyncStore.updateStateSyncRelayerEvent(eventData); err != nil {
		return fmt.Errorf("failed to move to sent: %d, %w", eventData.EventID, err)
	}

	// it is successful if there is no error or if it was successful first time we sent it but db update failed
	if err := ssr.sendTx(tx, eventData.EventID); err == nil || errors.Is(err, errAlreadyProcessedStateSync) {
		ssr.logger.Info("state sync relayer has successfully sent the event", "eventID", eventData.EventID)

		if err := ssr.state.StateSyncStore.removeStateSyncRelayerEvent(eventData.EventID); err != nil {
			ssr.logger.Error("failed to update store", "eventID", eventData.EventID, "err", err)
		}
	} else {
		// do not update database on error (change SentStatus for example)
		// event sending will be retried eventually after maxBlocksToWaitForResend blocks
		ssr.logger.Error("failed to execute tx", "eventID", eventData.EventID, "err", err)
	}

	return nil
}

func (ssr *stateSyncRelayerImpl) createTx(eventID uint64) (*ethgo.Transaction, error) {
	var sse *contractsapi.StateSync

	proof, err := ssr.store.GetStateSyncProof(eventID)
	if err != nil {
		return nil, err
	}

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

func (ssr *stateSyncRelayerImpl) sendTx(txn *ethgo.Transaction, eventID uint64) error {
	ssr.logger.Info("state sync relayer sending event", "eventID", eventID)

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
			return fmt.Errorf("Error: %w, Message: %s", errFailedToExecuteStateSync,
				string(stateSyncResult.Message))
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
