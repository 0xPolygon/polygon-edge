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

var (
	errUnknownStateSyncRelayerEvent = errors.New("unknown event from state receiver contract")

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

// StateSyncProofRetriever is an interface that exposes function for retrieving state sync proof
type StateSyncProofRetriever interface {
	GetStateSyncProof(stateSyncID uint64) (types.Proof, error)
}

var _ StateSyncRelayer = (*dummyStateSyncRelayer)(nil)

// dummyStateSyncRelayer is a dummy implementation of a StateSyncRelayer
type dummyStateSyncRelayer struct{}

func (d *dummyStateSyncRelayer) PostBlock(req *PostBlockRequest) error { return nil }
func (d *dummyStateSyncRelayer) Init() error                           { return nil }
func (d *dummyStateSyncRelayer) Close()                                {}

// EventSubscriber implementation
func (d *dummyStateSyncRelayer) GetLogFilters() map[types.Address][]types.Hash {
	return make(map[types.Address][]types.Hash)
}
func (d *dummyStateSyncRelayer) ProcessLog(header *types.Header, log *ethgo.Log, dbTx *bolt.Tx) error {
	return nil
}

var _ StateSyncRelayer = (*stateSyncRelayerImpl)(nil)

type stateSyncRelayerImpl struct {
	*relayerEventsProcessor

	txRelayer      txrelayer.TxRelayer
	key            ethgo.Key
	proofRetriever StateSyncProofRetriever
	logger         hclog.Logger

	notifyCh chan struct{}
	closeCh  chan struct{}
}

func newStateSyncRelayer(
	txRelayer txrelayer.TxRelayer,
	state *StateSyncStore,
	store StateSyncProofRetriever,
	blockchain blockchainBackend,
	key ethgo.Key,
	config *relayerConfig,
	logger hclog.Logger,
) *stateSyncRelayerImpl {
	relayer := &stateSyncRelayerImpl{
		txRelayer:      txRelayer,
		key:            key,
		proofRetriever: store,
		closeCh:        make(chan struct{}),
		notifyCh:       make(chan struct{}, 1),
		logger:         logger,
		relayerEventsProcessor: &relayerEventsProcessor{
			state:      state,
			logger:     logger,
			config:     config,
			blockchain: blockchain,
		},
	}

	relayer.relayerEventsProcessor.sendTx = relayer.sendTx

	return relayer
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

func (ssr stateSyncRelayerImpl) sendTx(events []*RelayerEventMetaData) error {
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
		To:    (*ethgo.Address)(&ssr.config.eventExecutionAddr),
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
	return map[types.Address][]types.Hash{
		contracts.StateReceiverContract: {
			types.Hash(new(contractsapi.StateSyncResultEvent).Sig()),
			types.Hash(new(contractsapi.NewCommitmentEvent).Sig()),
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
		newEvents := make([]*RelayerEventMetaData, lastID-firstID+1)

		for eventID := firstID; eventID <= lastID; eventID++ {
			newEvents[eventID-firstID] = &RelayerEventMetaData{EventID: eventID}
		}

		ssr.logger.Debug("new commitment event has arrived",
			"block", header.Number,
			"commitmentStartID", commitEvent.StartID,
			"commitmentEndID", commitEvent.EndID,
			"events", newEvents)

		return ssr.state.UpdateRelayerEvents(newEvents, nil, dbTx)

	case stateSyncResultEventSignature:
		_, err := stateSyncResultEvent.ParseLog(log)
		if err != nil {
			return err
		}

		eventID := stateSyncResultEvent.Counter.Uint64()

		if stateSyncResultEvent.Status {
			ssr.logger.Debug("state sync result event has been processed", "block", header.Number, "stateSyncID", eventID)

			return ssr.state.UpdateRelayerEvents(nil, []uint64{eventID}, dbTx)
		}

		ssr.logger.Debug("state sync result event failed to process", "block", header.Number,
			"stateSyncID", eventID, "reason", string(stateSyncResultEvent.Message))

		return nil

	default:
		return errUnknownStateSyncRelayerEvent
	}
}

func getBridgeTxRelayer(rpcEndpoint string, logger hclog.Logger) (txrelayer.TxRelayer, error) {
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
