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
	// defaultMaxBlocksToWaitForResend specifies how many blocks should be wait
	// in order to try again to send transaction
	defaultMaxBlocksToWaitForResend = uint64(30)
	// defaultMaxAttemptsToSend specifies how many sending retries for one transaction
	defaultMaxAttemptsToSend = 6
)

var (
	errFailedToExecuteStateSync    = errors.New("failed to execute state sync")
	errUnkownStateSyncRelayerEvent = errors.New("unknown event")
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
	EventID     uint64 `json:"eventID"`
	CountTries  uint64 `json:"countTries"`
	BlockNumber uint64 `json:"blockNumber"` // block when state sync is sent
	SentStatus  bool   `json:"sentStatus"`
}

func (ed StateSyncRelayerEventData) String() string {
	return fmt.Sprintf("%d", ed.EventID)
}

type stateSyncRelayerConfig struct {
	maxBlocksToWaitForResend uint64
	maxAttemptsToSend        uint64
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

	config *stateSyncRelayerConfig

	// eventsGetter gets contractsapi.NewCommitmentEvent and contractsapi.StateSyncResultEvent from blocks
	eventsGetter *eventsGetter[contractsapi.EventAbi]
}

func NewStateSyncRelayer(
	txRelayer txrelayer.TxRelayer,
	stateReceiverAddr types.Address,
	state *State,
	store stateSyncProofRetriever,
	blockchain blockchainBackend,
	key ethgo.Key,
	config *stateSyncRelayerConfig,
	logger hclog.Logger,
) StateSyncRelayer {
	if config == nil {
		config = &stateSyncRelayerConfig{
			maxBlocksToWaitForResend: defaultMaxBlocksToWaitForResend,
			maxAttemptsToSend:        defaultMaxAttemptsToSend,
		}
	}

	return &stateSyncRelayerImpl{
		txRelayer:  txRelayer,
		key:        key,
		store:      store,
		state:      state,
		closeCh:    make(chan struct{}),
		notifyCh:   make(chan struct{}, 1),
		blockchain: blockchain,
		eventsGetter: &eventsGetter[contractsapi.EventAbi]{
			blockchain: blockchain,
			isValidLogFn: func(l *types.Log) bool {
				return l.Address == stateReceiverAddr || l.Address == contracts.StateReceiverContract
			},
			parseEventFn: func(_ *types.Header, log *ethgo.Log) (contractsapi.EventAbi, bool, error) {
				var (
					commitEvent          contractsapi.NewCommitmentEvent
					stateSyncResultEvent contractsapi.StateSyncResultEvent
				)

				switch log.Topics[0] {
				case commitEvent.Sig():
					doesMatch, err := commitEvent.ParseLog(log)

					return &commitEvent, doesMatch, err
				case stateSyncResultEvent.Sig():
					doesMatch, err := stateSyncResultEvent.ParseLog(log)

					return &stateSyncResultEvent, doesMatch, err
				default:
					return nil, false, errUnkownStateSyncRelayerEvent
				}
			},
		},
		config: config,
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
				ssr.processEvents()
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
		return fmt.Errorf("failed to get state sync relayer data: %w", err)
	}

	// create default state data if this is the first time
	if state == nil {
		state = &StateSyncRelayerStateData{LastBlockNumber: 0}
	}

	allEvents, err := ssr.eventsGetter.getFromBlocks(state.LastBlockNumber, req.FullBlock)
	if err != nil {
		return fmt.Errorf("failed to retrieve events from block: %w", err)
	}

	newEvents := make([]*StateSyncRelayerEventData, 0)
	processedEventIDs := make([]uint64, 0)
	failedEventIDs := make([]uint64, 0)
	failedReasons := make([]string, 0)

	// process new events
	for _, genEvent := range allEvents {
		switch event := genEvent.(type) {
		case *contractsapi.NewCommitmentEvent:
			for i := event.StartID.Uint64(); i <= event.EndID.Uint64(); i++ {
				newEvents = append(newEvents, &StateSyncRelayerEventData{EventID: i})
			}

		case *contractsapi.StateSyncResultEvent:
			if event.Status {
				processedEventIDs = append(processedEventIDs, event.Counter.Uint64())
			} else {
				// maybe add failed events as new ones (that would overwrite in db)
				// newEvents = append(newEvents, &StateSyncRelayerEventData{EventID: event.Counter.Uint64()})
				failedEventIDs = append(failedEventIDs, event.Counter.Uint64())
				failedReasons = append(failedReasons, string(event.Message))
			}
		}
	}

	state.LastBlockNumber = req.FullBlock.Block.Number()

	err = ssr.state.StateSyncStore.updateStateSyncRelayerStateData(state, newEvents, processedEventIDs)
	if err != nil {
		return fmt.Errorf("state sync relayer insert state failed: %w", err)
	}

	ssr.logger.Info("state sync relayer updated state", "block", state.LastBlockNumber,
		"new", newEvents, "processed", processedEventIDs,
		"failed", failedEventIDs, "reasons", failedReasons)

	select {
	case ssr.notifyCh <- struct{}{}:
	default:
	}

	return nil
}

func (ssr *stateSyncRelayerImpl) processEvents() {
	// we need two events because the first one is possible tried maxAttemptsToSend times
	events, err := ssr.state.StateSyncStore.getAllAvailableEvents(2)
	if err != nil {
		ssr.logger.Error("retrieving events failed", "err", err)

		return
	}

	if len(events) == 0 {
		return // nothing to process
	}

	currentBlockNumber := ssr.blockchain.CurrentHeader().Number
	eventData := events[0]
	// return if we are still waiting for confirmation for the event
	if eventData.SentStatus && eventData.BlockNumber+ssr.config.maxBlocksToWaitForResend > currentBlockNumber {
		return
	}

	// if current event is processed to many times, remove it and try the next one
	if eventData.CountTries+1 > ssr.config.maxAttemptsToSend {
		err = ssr.state.StateSyncStore.removeStateSyncRelayerEvent(eventData.EventID)

		ssr.logger.Info("failed to send event too many times", "eventID", eventData.EventID, "err", err)
		// return if there is no next
		if len(events) == 1 {
			return
		}

		eventData = events[1] // next one to process
	}

	if err := ssr.sendEventTx(eventData, currentBlockNumber); err != nil {
		ssr.logger.Error("failed to send tx event", "event", eventData.EventID, "err", err)
	} else {
		ssr.logger.Info("event tx has been successfully sent", "event", eventData.EventID)
	}
}

func (ssr *stateSyncRelayerImpl) sendEventTx(eventData *StateSyncRelayerEventData, blockNumber uint64) error {
	eventData.CountTries++
	eventData.BlockNumber = blockNumber
	eventData.SentStatus = true

	if err := ssr.state.StateSyncStore.updateStateSyncRelayerEvent(eventData); err != nil {
		return fmt.Errorf("failed to update event: %w", err)
	}

	txn, err := ssr.createTx(eventData.EventID)
	if err != nil {
		return fmt.Errorf("failed to create tx: %w", err)
	}

	_, err = ssr.txRelayer.SendTransaction(txn, ssr.key)

	return err
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
