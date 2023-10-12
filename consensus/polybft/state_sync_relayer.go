package polybft

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
)

const (
	// maxBlocksToWaitForResend specifies how many blocks should be wait in order to try again to send transaction
	maxBlocksToWaitForResend = uint64(30)
	// maxAttemptsToSend specifies how many sending retries for one transaction
	maxAttemptsToSend = 6
)

var (
	errFailedToExecuteStateSync = errors.New("failed to execute state sync")
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
	txRelayer  txrelayer.TxRelayer
	key        ethgo.Key
	store      stateSyncProofRetriever
	state      *State
	logger     hclog.Logger
	blockchain blockchainBackend

	notifyCh chan struct{}
	closeCh  chan struct{}

	// eventsGetter gets contractsapi.NewCommitmentEvent from blocks
	eventsGetter *eventsGetter[*contractsapi.NewCommitmentEvent]
	// eventsGetter gets StateSyncResult events (missed or current) from blocks
	eventsGetterResults *eventsGetter[*contractsapi.StateSyncResultEvent]
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
		eventsGetterResults: &eventsGetter[*contractsapi.StateSyncResultEvent]{
			blockchain: blockchain,
			isValidLogFn: func(l *types.Log) bool {
				return l.Address == contracts.StateReceiverContract
			},
			parseEventFn: func(h *types.Header, l *ethgo.Log) (*contractsapi.StateSyncResultEvent, bool, error) {
				var stateSyncResultEvent contractsapi.StateSyncResultEvent
				matches, err := stateSyncResultEvent.ParseLog(l)

				return &stateSyncResultEvent, matches, err
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
				if ed, err := ssr.processEvent(); err != nil {
					ssr.logger.Error("failed to process event", "event", ed.EventID, "err", err)
				} else if ed != nil {
					ssr.logger.Info("event has been successfully sent", "event", ed.EventID)
				}
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

	resultEvents, err := ssr.eventsGetterResults.getFromBlocks(state.LastBlockNumber, req.FullBlock)
	if err != nil {
		return fmt.Errorf("failed to retrieve processed state sync result events from block: %w", err)
	}

	processedStateSyncEventIDs := make([]uint64, 0, len(resultEvents))
	failedStateSyncEventIDs := make([]uint64, 0, len(resultEvents))
	failedReasons := make([]string, 0, len(resultEvents))

	for _, event := range resultEvents {
		if event.Status {
			processedStateSyncEventIDs = append(processedStateSyncEventIDs, event.Counter.Uint64())
		} else {
			failedStateSyncEventIDs = append(failedStateSyncEventIDs, event.Counter.Uint64())
			failedReasons = append(failedReasons, string(event.Message))
		}
	}

	events, err := ssr.eventsGetter.getFromBlocks(state.LastBlockNumber, req.FullBlock)
	if err != nil {
		return fmt.Errorf("state sync relayer: %w", err)
	}

	state.LastBlockNumber = req.FullBlock.Block.Number()

	if err = ssr.state.StateSyncStore.updateStateSyncRelayerStateData(
		state,
		getConvertedEvents(events), processedStateSyncEventIDs); err != nil {
		return fmt.Errorf("state sync relayer insert state failed: %w", err)
	}

	// maybe change state of failed events to Sent=false
	ssr.logger.Info("state sync relayer updated state", "block", state.LastBlockNumber,
		"processed", processedStateSyncEventIDs,
		"failed", failedStateSyncEventIDs, "reasons", failedReasons)

	select {
	case ssr.notifyCh <- struct{}{}:
	default:
	}

	return nil
}

func (ssr *stateSyncRelayerImpl) processEvent() (*StateSyncRelayerEventData, error) {
	eventData, err := ssr.state.StateSyncStore.getNextEvent()
	if err != nil {
		return eventData, err
	} else if eventData == nil {
		return nil, nil // nothing to process right now
	} else if eventData.SentStatus &&
		eventData.BlockNumber+maxBlocksToWaitForResend > ssr.blockchain.CurrentHeader().Number {
		return nil, nil // nothing to process right now, still waiting for confirmation
	} else if eventData.CountTries+1 > maxAttemptsToSend {
		_ = ssr.state.StateSyncStore.removeStateSyncRelayerEvent(eventData.EventID)

		ssr.logger.Info("failed to send event too many times", "eventID", eventData.EventID)

		return ssr.processEvent() // process next
	}

	ssr.logger.Info("state sync relayer processing event", "eventID", eventData.EventID)

	txn, err := ssr.createTx(eventData.EventID)
	if err != nil {
		_ = ssr.state.StateSyncStore.updateStateSyncRelayerEvent(eventData)

		return eventData, fmt.Errorf("failed to create tx: %w", err)
	}

	eventData.CountTries++
	eventData.BlockNumber = ssr.blockchain.CurrentHeader().Number
	eventData.SentStatus = true

	if err := ssr.state.StateSyncStore.updateStateSyncRelayerEvent(eventData); err != nil {
		return eventData, fmt.Errorf("failed to move to sent: %w", err)
	}

	ssr.logger.Info("state sync relayer sending event", "eventID", eventData.EventID)

	_, err = ssr.txRelayer.SendTransaction(txn, ssr.key)

	return eventData, err
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
		txrelayer.WithIPAddress(rpcEndpoint), txrelayer.WithNumRetries(-1),
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
