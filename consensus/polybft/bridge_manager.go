package polybft

import (
	"fmt"
	"path"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Ethernal-Tech/blockchain-event-tracker/store"
	"github.com/Ethernal-Tech/blockchain-event-tracker/tracker"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	bolt "go.etcd.io/bbolt"
)

type proofType int

const (
	StateSync proofType = iota
	Exit
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
	stateSyncEventSig           = new(contractsapi.StateSyncedEvent).Sig()
	checkpointSubmittedEventSig = new(contractsapi.CheckpointSubmittedEvent).Sig()
	exitProcessedEventSig       = new(contractsapi.ExitProcessedEvent).Sig()
)

// BridgeBackend is an interface that defines functions required by bridge components
type BridgeBackend interface {
	Runtime
	StateSyncProofRetriever
	ExitEventProofRetriever
}

// RelayerEventMetaData keeps information about a relayer event
type RelayerEventMetaData struct {
	EventID     uint64 `json:"eventID"`
	CountTries  uint64 `json:"countTries"`
	BlockNumber uint64 `json:"blockNumber"` // block when event is sent
	SentStatus  bool   `json:"sentStatus"`
}

func (ed RelayerEventMetaData) String() string {
	return fmt.Sprintf("%d", ed.EventID)
}

// relayerConfig is a struct that holds the relayer configuration
type relayerConfig struct {
	maxBlocksToWaitForResend uint64
	maxAttemptsToSend        uint64
	maxEventsPerBatch        uint64
	eventExecutionAddr       types.Address
}

// eventTrackerConfig is a struct that holds the event tracker configuration
type eventTrackerConfig struct {
	consensus.EventTracker

	jsonrpcAddr           string
	startBlock            uint64
	stateSenderAddr       types.Address
	checkpointManagerAddr types.Address
	exitHelperAddr        types.Address
	trackerPollInterval   time.Duration
}

// RelayerState is an interface that defines functions that a relayer store has to implement
type RelayerState interface {
	GetAllAvailableRelayerEvents(limit int) (result []*RelayerEventMetaData, err error)
	UpdateRelayerEvents(events []*RelayerEventMetaData, removeIDs []uint64, dbTx *bolt.Tx) error
}

// relayerEventsProcessor is a parent struct of both state sync and exit relayer
// that holds functions common to both relayers
type relayerEventsProcessor struct {
	logger     hclog.Logger
	state      RelayerState
	blockchain blockchainBackend

	config *relayerConfig
	sendTx func([]*RelayerEventMetaData) error
}

// ProcessEvents processes all relayer events that were either successfully or unsuccessfully executed
// and executes all the events that can be executed in regards to relayerConfig
func (r *relayerEventsProcessor) processEvents() {
	// we need twice as batch size because events from first batch are possible already sent maxAttemptsToSend times
	events, err := r.state.GetAllAvailableRelayerEvents(int(r.config.maxEventsPerBatch) * 2)
	if err != nil {
		r.logger.Error("retrieving events failed", "err", err)

		return
	}

	if len(events) == 0 {
		return
	}

	removedEventIDs := make([]uint64, 0, len(events))
	sendingEvents := make([]*RelayerEventMetaData, 0, len(events))
	currentBlockNumber := r.blockchain.CurrentHeader().Number

	// check already processed events
	for _, event := range events {
		// quit if we are still waiting for some old event confirmation (there is no parallelization right now!)
		if event.SentStatus && event.BlockNumber+r.config.maxBlocksToWaitForResend > currentBlockNumber {
			return
		}

		// remove event if it is processed too many times
		if event.CountTries+1 > r.config.maxAttemptsToSend {
			removedEventIDs = append(removedEventIDs, event.EventID)
		} else {
			event.CountTries++
			event.BlockNumber = currentBlockNumber
			event.SentStatus = true

			sendingEvents = append(sendingEvents, event)
			if len(sendingEvents) == int(r.config.maxEventsPerBatch) {
				break
			}
		}
	}

	// update state only if needed
	if len(sendingEvents)+len(removedEventIDs) > 0 {
		r.logger.Debug("updating relayer events storage", "events", sendingEvents, "removed", removedEventIDs)

		if err := r.state.UpdateRelayerEvents(sendingEvents, removedEventIDs, nil); err != nil {
			r.logger.Error("updating relayer events storage failed",
				"events", sendingEvents, "removed", removedEventIDs, "err", err)

			return
		}
	}

	// send tx only if needed
	if len(sendingEvents) > 0 {
		if err := r.sendTx(sendingEvents); err != nil {
			r.logger.Error("failed to send relayer tx", "block", currentBlockNumber, "events", sendingEvents, "err", err)
		} else {
			r.logger.Debug("relayer tx has been successfully sent", "block", currentBlockNumber, "events", sendingEvents)
		}
	}
}

// BridgeManager is an interface that defines functions that a bridge manager must implement
type BridgeManager interface {
	tracker.EventSubscriber

	Close()
	PostBlockAsync(req *PostBlockRequest)
	PostBlock(req *PostBlockRequest) error
	PostEpoch(req *PostEpochRequest) error
	BuildExitEventRoot(epoch uint64) (types.Hash, error)
	GenerateProof(eventID uint64, pType proofType) (types.Proof, error)
	Commitment(pendingBlockNumber uint64) (*CommitmentMessageSigned, error)
}

var _ BridgeManager = (*dummyBridgeManager)(nil)

type dummyBridgeManager struct{}

func (d *dummyBridgeManager) Close()                                {}
func (d *dummyBridgeManager) PostBlockAsync(req *PostBlockRequest)  {}
func (d *dummyBridgeManager) AddLog(log *ethgo.Log) error           { return nil }
func (d *dummyBridgeManager) PostBlock(req *PostBlockRequest) error { return nil }
func (d *dummyBridgeManager) PostEpoch(req *PostEpochRequest) error { return nil }
func (d *dummyBridgeManager) BuildExitEventRoot(epoch uint64) (types.Hash, error) {
	return types.ZeroHash, nil
}
func (d *dummyBridgeManager) Commitment(pendingBlockNumber uint64) (*CommitmentMessageSigned, error) {
	return nil, nil
}
func (d *dummyBridgeManager) GenerateProof(eventID uint64, pType proofType) (types.Proof, error) {
	return types.Proof{}, nil
}

var _ BridgeManager = (*bridgeManager)(nil)

// bridgeManager is a struct that manages different bridge components
// such as handling and executing bridge events
type bridgeManager struct {
	checkpointManager CheckpointManager
	stateSyncManager  StateSyncManager
	stateSyncRelayer  StateSyncRelayer
	exitEventRelayer  ExitRelayer

	eventTrackerConfig *eventTrackerConfig
	logger             hclog.Logger
}

// newBridgeManager creates a new instance of bridgeManager
func newBridgeManager(
	bridgeBackend BridgeBackend,
	runtimeConfig *runtimeConfig,
	eventProvider *EventProvider,
	logger hclog.Logger) (BridgeManager, error) {
	if !runtimeConfig.GenesisConfig.IsBridgeEnabled() {
		return &dummyBridgeManager{}, nil
	}

	stateSenderAddr := runtimeConfig.GenesisConfig.Bridge.StateSenderAddr
	bridgeManager := &bridgeManager{
		logger: logger.Named("bridge-manager"),
		eventTrackerConfig: &eventTrackerConfig{
			EventTracker:          *runtimeConfig.eventTracker,
			stateSenderAddr:       stateSenderAddr,
			exitHelperAddr:        runtimeConfig.GenesisConfig.Bridge.ExitHelperAddr,
			checkpointManagerAddr: runtimeConfig.GenesisConfig.Bridge.CheckpointManagerAddr,
			jsonrpcAddr:           runtimeConfig.GenesisConfig.Bridge.JSONRPCEndpoint,
			startBlock:            runtimeConfig.GenesisConfig.Bridge.EventTrackerStartBlocks[stateSenderAddr],
			trackerPollInterval:   runtimeConfig.GenesisConfig.BlockTrackerPollInterval.Duration,
		},
	}

	if err := bridgeManager.initStateSyncManager(bridgeBackend, runtimeConfig, logger); err != nil {
		return nil, err
	}

	if err := bridgeManager.initCheckpointManager(eventProvider, runtimeConfig, logger); err != nil {
		return nil, err
	}

	if err := bridgeManager.initStateSyncRelayer(bridgeBackend, eventProvider, runtimeConfig, logger); err != nil {
		return nil, err
	}

	if err := bridgeManager.initExitRelayer(bridgeBackend, runtimeConfig, logger); err != nil {
		return nil, err
	}

	if err := bridgeManager.initTracker(runtimeConfig); err != nil {
		return nil, fmt.Errorf("failed to init event tracker. Error: %w", err)
	}

	return bridgeManager, nil
}

// PostBlock is a function executed on every block finalization (either by consensus or syncer)
func (b *bridgeManager) PostBlock(req *PostBlockRequest) error {
	if err := b.stateSyncManager.PostBlock(req); err != nil {
		return fmt.Errorf("failed to execute post block in state sync manager. Err: %w", err)
	}

	if err := b.stateSyncRelayer.PostBlock(req); err != nil {
		return fmt.Errorf("failed to execute post block in state sync relayer. Err: %w", err)
	}

	if err := b.exitEventRelayer.PostBlock(req); err != nil {
		return fmt.Errorf("failed to execute post block in exit event relayer. Err: %w", err)
	}

	b.checkpointManager.PostBlock(req)

	return nil
}

// PostEpoch is a function executed on epoch ending / start of new epoch
func (b *bridgeManager) PostEpoch(req *PostEpochRequest) error {
	if err := b.stateSyncManager.PostEpoch(req); err != nil {
		return fmt.Errorf("failed to execute post epoch in state sync manager. Error: %w", err)
	}

	return nil
}

// BuildExitEventRoot builds exit event root for given epoch
func (b *bridgeManager) BuildExitEventRoot(epoch uint64) (types.Hash, error) {
	return b.checkpointManager.BuildEventRoot(epoch)
}

// Commitment returns the pending signed state sync commitment
func (b *bridgeManager) Commitment(pendingBlockNumber uint64) (*CommitmentMessageSigned, error) {
	return b.stateSyncManager.Commitment(pendingBlockNumber)
}

// GenerateProof generates proof for a specific event type
func (b *bridgeManager) GenerateProof(eventID uint64, pType proofType) (types.Proof, error) {
	switch pType {
	case StateSync:
		return b.stateSyncManager.GetStateSyncProof(eventID)
	case Exit:
		return b.checkpointManager.GenerateExitProof(eventID)
	default:
		return types.Proof{}, fmt.Errorf("unknown proof type requested: %v", pType)
	}
}

// PostBlockAsync is called on finalization of each block (either from consensus or syncer)
// but it doesn't require return of any kind, and is done asynchronously
func (b *bridgeManager) PostBlockAsync(req *PostBlockRequest) {
	// we will do PostBlock on checkpoint manager at the end, because it only
	// sends a checkpoint in a separate routine. It doesn't do any db operations
	b.checkpointManager.PostBlock(req)
}

// close stops ongoing go routines in the manager
func (b *bridgeManager) Close() {
	b.stateSyncRelayer.Close()
	b.exitEventRelayer.Close()
}

// initStateSyncManager initializes state sync manager
// if bridge is not enabled, then a dummy state sync manager will be used
func (b *bridgeManager) initStateSyncManager(
	bridgeBackend BridgeBackend,
	runtimeConfig *runtimeConfig,
	logger hclog.Logger) error {
	stateSyncManager := newStateSyncManager(
		logger.Named("state-sync-manager"),
		runtimeConfig.State,
		&stateSyncConfig{
			key:               runtimeConfig.Key,
			dataDir:           runtimeConfig.DataDir,
			topic:             runtimeConfig.bridgeTopic,
			maxCommitmentSize: maxCommitmentSize,
		},
		bridgeBackend,
	)

	b.stateSyncManager = stateSyncManager

	return b.stateSyncManager.Init()
}

// initCheckpointManager initializes checkpoint manager
// if bridge is not enabled, then a dummy checkpoint manager will be used
func (b *bridgeManager) initCheckpointManager(
	eventProvider *EventProvider,
	runtimeConfig *runtimeConfig,
	logger hclog.Logger) error {
	log := logger.Named("checkpoint_manager")

	txRelayer, err := txrelayer.NewTxRelayer(
		txrelayer.WithIPAddress(runtimeConfig.GenesisConfig.Bridge.JSONRPCEndpoint),
		txrelayer.WithWriter(log.StandardWriter(&hclog.StandardLoggerOptions{})))
	if err != nil {
		return err
	}

	b.checkpointManager = newCheckpointManager(
		wallet.NewEcdsaSigner(runtimeConfig.Key),
		runtimeConfig.GenesisConfig.Bridge.CheckpointManagerAddr,
		txRelayer,
		runtimeConfig.blockchain,
		runtimeConfig.polybftBackend,
		log,
		runtimeConfig.State)

	eventProvider.Subscribe(b.checkpointManager)

	return nil
}

// initStateSyncRelayer initializes state sync relayer
// if not enabled, then a dummy state sync relayer will be used
func (b *bridgeManager) initStateSyncRelayer(
	bridgeBackend BridgeBackend,
	eventProvider *EventProvider,
	runtimeConfig *runtimeConfig,
	logger hclog.Logger) error {
	if runtimeConfig.consensusConfig.IsRelayer {
		txRelayer, err := getBridgeTxRelayer(runtimeConfig.consensusConfig.RPCEndpoint, logger)
		if err != nil {
			return err
		}

		b.stateSyncRelayer = newStateSyncRelayer(
			txRelayer,
			runtimeConfig.State.StateSyncStore,
			bridgeBackend,
			runtimeConfig.blockchain,
			wallet.NewEcdsaSigner(runtimeConfig.Key),
			&relayerConfig{
				maxBlocksToWaitForResend: defaultMaxBlocksToWaitForResend,
				maxAttemptsToSend:        defaultMaxAttemptsToSend,
				maxEventsPerBatch:        defaultMaxEventsPerBatch,
				eventExecutionAddr:       contracts.StateReceiverContract,
			},
			logger.Named("state_sync_relayer"))
	} else {
		b.stateSyncRelayer = &dummyStateSyncRelayer{}
	}

	eventProvider.Subscribe(b.stateSyncRelayer)

	return b.stateSyncRelayer.Init()
}

// initStateSyncRelayer initializes exit event relayer
// if not enabled, then a dummy exit event relayer will be used
func (b *bridgeManager) initExitRelayer(
	bridgeBackend BridgeBackend,
	runtimeConfig *runtimeConfig,
	logger hclog.Logger) error {
	if runtimeConfig.consensusConfig.IsRelayer {
		txRelayer, err := getBridgeTxRelayer(runtimeConfig.GenesisConfig.Bridge.JSONRPCEndpoint, logger)
		if err != nil {
			return err
		}

		b.exitEventRelayer = newExitRelayer(
			txRelayer,
			wallet.NewEcdsaSigner(runtimeConfig.Key),
			bridgeBackend,
			runtimeConfig.blockchain,
			runtimeConfig.State.ExitStore,
			&relayerConfig{
				maxBlocksToWaitForResend: defaultMaxBlocksToWaitForResend,
				maxAttemptsToSend:        defaultMaxAttemptsToSend,
				maxEventsPerBatch:        defaultMaxEventsPerBatch,
				eventExecutionAddr:       runtimeConfig.GenesisConfig.Bridge.ExitHelperAddr,
			},
			logger.Named("exit_relayer"))
	} else {
		b.exitEventRelayer = &dummyExitRelayer{}
	}

	return b.exitEventRelayer.Init()
}

// initTracker starts a new event tracker (to receive bridge events)
func (b *bridgeManager) initTracker(runtimeConfig *runtimeConfig) error {
	store, err := store.NewBoltDBEventTrackerStore(path.Join(runtimeConfig.DataDir, "/bridge.db"))
	if err != nil {
		return err
	}

	eventTracker, err := tracker.NewEventTracker(
		&tracker.EventTrackerConfig{
			EventSubscriber:        b,
			Logger:                 b.logger,
			RPCEndpoint:            b.eventTrackerConfig.jsonrpcAddr,
			SyncBatchSize:          b.eventTrackerConfig.EventTracker.SyncBatchSize,
			NumBlockConfirmations:  b.eventTrackerConfig.EventTracker.NumBlockConfirmations,
			NumOfBlocksToReconcile: b.eventTrackerConfig.EventTracker.NumOfBlocksToReconcile,
			PollInterval:           b.eventTrackerConfig.trackerPollInterval,
			LogFilter: map[ethgo.Address][]ethgo.Hash{
				ethgo.Address(b.eventTrackerConfig.stateSenderAddr):       {stateSyncEventSig},
				ethgo.Address(b.eventTrackerConfig.checkpointManagerAddr): {checkpointSubmittedEventSig},
				ethgo.Address(b.eventTrackerConfig.exitHelperAddr):        {exitProcessedEventSig},
			},
		},
		store, b.eventTrackerConfig.startBlock,
	)

	if err != nil {
		return err
	}

	return eventTracker.Start()
}

// AddLog saves the received log from event tracker if it matches a state sync event ABI
func (b *bridgeManager) AddLog(eventLog *ethgo.Log) error {
	switch eventLog.Topics[0] {
	case stateSyncEventSig:
		return b.stateSyncManager.AddLog(eventLog)
	case checkpointSubmittedEventSig:
		return b.exitEventRelayer.AddLog(eventLog)
	case exitProcessedEventSig:
		return b.exitEventRelayer.AddLog(eventLog)
	default:
		b.logger.Error("Unknown event log receiver from event tracker")

		return nil
	}
}
