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
	bolt "go.etcd.io/bbolt"
)

const (
	// defaultMaxBlocksToWaitForResend specifies how many blocks should be wait
	// in order to try again to send transaction
	defaultMaxBlocksToWaitForResend = uint64(30)
	// defaultMaxAttemptsToSend specifies how many sending retries for one transaction
	defaultMaxAttemptsToSend = uint64(15)
	// defaultMaxEventsPerBatch specifies maximum events per one batchExecute tx
	defaultMaxEventsPerBatch = uint64(10)
)

var (
	errFailedToExecuteStateSync     = errors.New("failed to execute state sync")
	errUnknownStateSyncRelayerEvent = errors.New("unknown event")

	commitmentEventSignature      = new(contractsapi.NewCommitmentEvent).Sig()
	stateSyncResultEventSignature = new(contractsapi.StateSyncResultEvent).Sig()
)

// StateSyncRelayer is an interface that defines functions for state sync relayer
type StateSyncRelayer interface {
	EventSubscriber
	PostBlock(req *PostBlockRequest) error
	Init() error
	Close()
}

// stateSyncProofRetriever is an interface that exposes function for retrieving state sync proof
type stateSyncProofRetriever interface {
	GetStateSyncProof(stateSyncID uint64) (types.Proof, error)
}

var _ StateSyncRelayer = (*dummyStateSyncRelayer)(nil)

// dummyStateSyncRelayer is a dummy implementation of a StateSyncRelayer
type dummyStateSyncRelayer struct{}

func (d *dummyStateSyncRelayer) PostBlock(req *PostBlockRequest) error { return nil }

func (d *dummyStateSyncRelayer) Init() error { return nil }
func (d *dummyStateSyncRelayer) Close()      {}

// EventSubscriber implementation
func (d *dummyStateSyncRelayer) GetLogFilters() map[types.Address][]types.Hash {
	return make(map[types.Address][]types.Hash)
}
func (d *dummyStateSyncRelayer) ProcessLog(header *types.Header, log *ethgo.Log, dbTx *bolt.Tx) error {
	return nil
}

var _ StateSyncRelayer = (*stateSyncRelayerImpl)(nil)

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
	txRelayer      txrelayer.TxRelayer
	key            ethgo.Key
	proofRetriever stateSyncProofRetriever
	state          *StateSyncStore
	logger         hclog.Logger
	blockchain     blockchainBackend

	notifyCh chan struct{}
	closeCh  chan struct{}

	config *stateSyncRelayerConfig
}

func NewStateSyncRelayer(
	txRelayer txrelayer.TxRelayer,
	stateReceiverAddr types.Address,
	state *StateSyncStore,
	store stateSyncProofRetriever,
	blockchain blockchainBackend,
	key ethgo.Key,
	config *stateSyncRelayerConfig,
	logger hclog.Logger,
) *stateSyncRelayerImpl {
	if config == nil {
		config = &stateSyncRelayerConfig{
			maxBlocksToWaitForResend: defaultMaxBlocksToWaitForResend,
			maxAttemptsToSend:        defaultMaxAttemptsToSend,
			maxEventsPerBatch:        defaultMaxEventsPerBatch,
		}
	}

	return &stateSyncRelayerImpl{
		txRelayer:      txRelayer,
		key:            key,
		proofRetriever: store,
		state:          state,
		closeCh:        make(chan struct{}),
		notifyCh:       make(chan struct{}, 1),
		blockchain:     blockchain,
		config:         config,
		logger:         logger,
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
	select {
	case ssr.notifyCh <- struct{}{}:
	default:
	}

	return nil
}

func (ssr *stateSyncRelayerImpl) processEvents() {
	// we need twice as batch size because events from first batch are possible already sent maxAttemptsToSend times
	events, err := ssr.state.getAllAvailableEvents(int(ssr.config.maxEventsPerBatch) * 2)
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

		if err := ssr.state.updateStateSyncRelayerEvents(sendingEvents, removedEventIDs, nil); err != nil {
			ssr.logger.Error("updating events failed",
				"events", sendingEvents, "removed", removedEventIDs, "err", err)

			return
		}
	}

	// send tx only if needed
	if len(sendingEvents) > 0 {
		if err := ssr.sendTx(sendingEvents); err != nil {
			ssr.logger.Error("failed to send tx", "block", currentBlockNumber, "events", sendingEvents, "err", err)
		} else {
			ssr.logger.Info("tx has been successfully sent", "block", currentBlockNumber, "events", sendingEvents)
		}
	}
}

func (ssr *stateSyncRelayerImpl) sendTx(events []*StateSyncRelayerEventData) error {
	proofs := make([][]types.Hash, len(events))
	objs := make([]*contractsapi.StateSync, len(events))

	for i, event := range events {
		proof, err := ssr.proofRetriever.GetStateSyncProof(event.EventID)
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

// EventSubscriber implementation

// GetLogFilters returns a map of log filters for getting desired events,
// where the key is the address of contract that emits desired events,
// and the value is a slice of signatures of events we want to get.
// This function is the implementation of EventSubscriber interface
func (ssr *stateSyncRelayerImpl) GetLogFilters() map[types.Address][]types.Hash {
	var (
		stateSyncResultEvent contractsapi.StateSyncResultEvent
		newCommitmentEvent   contractsapi.NewCommitmentEvent
	)

	return map[types.Address][]types.Hash{
		contracts.StateReceiverContract: {
			types.Hash(stateSyncResultEvent.Sig()),
			types.Hash(newCommitmentEvent.Sig()),
		},
	}
}

// ProcessLog is the implementation of EventSubscriber interface,
// used to handle a log defined in GetLogFilters, provided by event provider
func (ssr *stateSyncRelayerImpl) ProcessLog(header *types.Header, log *ethgo.Log, dbTx *bolt.Tx) error {
	var (
		commitEvent          contractsapi.NewCommitmentEvent
		stateSyncResultEvent contractsapi.StateSyncResultEvent
	)

	switch log.Topics[0] {
	case commitmentEventSignature:
		_, err := commitEvent.ParseLog(log)
		if err != nil {
			return err
		}

		firstID, lastID := commitEvent.StartID.Uint64(), commitEvent.EndID.Uint64()
		newEvents := make([]*StateSyncRelayerEventData, lastID-firstID+1)

		for eventID := firstID; eventID <= lastID; eventID++ {
			newEvents[eventID-firstID] = &StateSyncRelayerEventData{EventID: eventID}
		}

		ssr.logger.Info("new events has been arrived", "block", header.Number, "events", newEvents)

		return ssr.state.updateStateSyncRelayerEvents(newEvents, nil, dbTx)

	case stateSyncResultEventSignature:
		_, err := stateSyncResultEvent.ParseLog(log)
		if err != nil {
			return err
		}

		eventID := stateSyncResultEvent.Counter.Uint64()

		if stateSyncResultEvent.Status {
			ssr.logger.Info("event has been processed", "block", header.Number, "event", eventID)

			return ssr.state.updateStateSyncRelayerEvents(nil, []uint64{eventID}, dbTx)
		}

		ssr.logger.Info("event has been failed to process", "block", header.Number,
			"event", eventID, "reason", string(stateSyncResultEvent.Message))

		return nil

	default:
		return errUnknownStateSyncRelayerEvent
	}
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
