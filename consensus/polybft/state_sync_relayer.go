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
	// defaultMaxEventsPerBatch specifies maximum events per one batchExecute tx
	defaultMaxEventsPerBatch = 10
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
	maxEventsPerBatch        uint64
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
			maxEventsPerBatch:        defaultMaxEventsPerBatch,
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

	// process all the events
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
	// we need twice as batch size because events from first batch are possible already sent maxAttemptsToSend times
	events, err := ssr.state.StateSyncStore.getAllAvailableEvents(int(ssr.config.maxEventsPerBatch) * 2)
	if err != nil {
		ssr.logger.Error("retrieving events failed", "err", err)

		return
	}

	removedEventIDs := []uint64{}
	sendingEvents := []*StateSyncRelayerEventData{}
	currentBlockNumber := ssr.blockchain.CurrentHeader().Number

	// check already processed events
	for _, evnt := range events {
		// quit if we are still waiting for some old event confirmation (there is no paralelization right now!)
		if evnt.SentStatus && evnt.BlockNumber+ssr.config.maxBlocksToWaitForResend > currentBlockNumber {
			return
		}

		// remove event if it is processed too many times
		if evnt.CountTries+1 > ssr.config.maxAttemptsToSend {
			removedEventIDs = append(removedEventIDs, evnt.EventID)
		} else {
			evnt.CountTries++
			evnt.BlockNumber = currentBlockNumber
			evnt.SentStatus = true

			sendingEvents = append(sendingEvents, evnt)
			if len(sendingEvents) == int(ssr.config.maxEventsPerBatch) {
				break
			}
		}
	}

	// update state only if needed
	if len(sendingEvents)+len(removedEventIDs) > 0 {
		ssr.logger.Info("sending events", "events", sendingEvents, "removed", removedEventIDs)

		if err := ssr.state.StateSyncStore.updateStateSyncRelayerEvents(sendingEvents, removedEventIDs); err != nil {
			ssr.logger.Error("updating events failed",
				"events", sendingEvents, "removed", removedEventIDs, "err", err)

			return
		}
	}

	// send tx only if needed
	if len(sendingEvents) > 0 {
		if err := ssr.sendTx(sendingEvents); err != nil {
			ssr.logger.Error("failed to send tx", "events", sendingEvents, "err", err)
		} else {
			ssr.logger.Info("tx has been successfully sent", "events", sendingEvents)
		}
	}
}

func (ssr *stateSyncRelayerImpl) sendTx(events []*StateSyncRelayerEventData) error {
	proofs := make([][]types.Hash, len(events))
	objs := make([]*contractsapi.StateSync, len(events))

	for i, event := range events {
		proof, err := ssr.store.GetStateSyncProof(event.EventID)
		if err != nil {
			return fmt.Errorf("failed to get proof for %d: %w", event.EventID, err)
		}

		// since state sync event is a map in the jsonrpc response,
		// to not have custom logic of converting the map to state sync event
		// json encoding is used, since it manages to successfully unmarshal the
		// event from the marshaled map
		raw, err := json.Marshal(proof.Metadata["StateSync"])
		if err != nil {
			return fmt.Errorf("failed to marshal event %d: %w", event.EventID, err)
		}

		if err = json.Unmarshal(raw, &objs[i]); err != nil {
			return fmt.Errorf("failed to unmarshal event %d: %w", event.EventID, err)
		}

		proofs[i] = proof.Data
	}

	input, err := (&contractsapi.BatchExecuteStateReceiverFn{
		Proofs: proofs,
		Objs:   objs,
	}).EncodeAbi()
	if err != nil {
		return err
	}

	// send batchExecute state sync
	_, err = ssr.txRelayer.SendTransaction(&ethgo.Transaction{
		From:  ssr.key.Address(),
		To:    (*ethgo.Address)(&contracts.StateReceiverContract),
		Gas:   types.StateTransactionGasLimit,
		Input: input,
	}, ssr.key)

	return err
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
