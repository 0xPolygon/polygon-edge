package polybft

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
)

var errUnknownExitEvent = errors.New("unknown event from exit helper or checkpoint manager")

// ExitRelayer is an interface that defines functions of an exit event handler and executioner
type ExitRelayer interface {
	Close()
	Init() error
	AddLog(eventLog *ethgo.Log) error
	PostBlock(req *PostBlockRequest) error
}

var _ ExitRelayer = (*dummyExitRelayer)(nil)

type dummyExitRelayer struct{}

func (d *dummyExitRelayer) Close()                                {}
func (d *dummyExitRelayer) Init() error                           { return nil }
func (d *dummyExitRelayer) AddLog(eventLog *ethgo.Log) error      { return nil }
func (d *dummyExitRelayer) PostBlock(req *PostBlockRequest) error { return nil }

// ExitEventProofRetriever is an interface that exposes function for retrieving exit proof
type ExitEventProofRetriever interface {
	GenerateExitProof(exitID uint64) (types.Proof, error)
}

var _ ExitRelayer = (*exitRelayer)(nil)

// exitRelayer handles checkpoint submitted events and executes exit events
type exitRelayer struct {
	*relayerEventsProcessor

	key            crypto.Key
	proofRetriever ExitEventProofRetriever
	txRelayer      txrelayer.TxRelayer
	logger         hclog.Logger
	exitStore      *ExitStore

	notifyCh chan struct{}
	closeCh  chan struct{}
}

// newExitRelayer creates a new instance of exitRelayer
func newExitRelayer(
	txRelayer txrelayer.TxRelayer,
	key crypto.Key,
	proofRetriever ExitEventProofRetriever,
	blockchain blockchainBackend,
	exitStore *ExitStore,
	config *relayerConfig,
	logger hclog.Logger) *exitRelayer {
	relayer := &exitRelayer{
		key:            key,
		logger:         logger,
		exitStore:      exitStore,
		txRelayer:      txRelayer,
		proofRetriever: proofRetriever,
		closeCh:        make(chan struct{}),
		notifyCh:       make(chan struct{}, 1),
		relayerEventsProcessor: &relayerEventsProcessor{
			config:     config,
			logger:     logger,
			state:      exitStore,
			blockchain: blockchain,
		},
	}

	relayer.relayerEventsProcessor.sendTx = relayer.sendTx

	return relayer
}

// Init starts the exit relayer go routine
func (e *exitRelayer) Init() error {
	// start consumer
	go func() {
		for {
			select {
			case <-e.closeCh:
				return
			case <-e.notifyCh:
				e.processEvents()
			}
		}
	}()

	return nil
}

// Close closes the running go routine
func (e *exitRelayer) Close() {
	close(e.closeCh)
}

// PostBlock is a function called on finalization of each block (through consensus or syncer)
func (e *exitRelayer) PostBlock(req *PostBlockRequest) error {
	select {
	case e.notifyCh <- struct{}{}:
	default:
	}

	return nil
}

// AddLog handles the received log from event tracker if it matches
// a checkpoint submitted event ABI, or exit processed event ABI
func (e *exitRelayer) AddLog(eventLog *ethgo.Log) error {
	var (
		checkpointSubmittedEvent contractsapi.CheckpointSubmittedEvent
		exitProcessedEvent       contractsapi.ExitProcessedEvent
	)

	switch eventLog.Topics[0] {
	case checkpointSubmittedEventSig:
		_, err := checkpointSubmittedEvent.ParseLog(eventLog)
		if err != nil {
			e.logger.Error("could not decode checkpoint submitted event", "err", err)

			return err
		}

		e.logger.Debug(
			"checkpoint submitted event arrived",
			"checkpointEpoch", checkpointSubmittedEvent.Epoch,
			"checkpointBlockNumber", checkpointSubmittedEvent.BlockNumber,
			"eventRoot", checkpointSubmittedEvent.EventRoot,
		)

		exitEvents, err := e.exitStore.getExitEventsForProof(
			checkpointSubmittedEvent.Epoch.Uint64(),
			checkpointSubmittedEvent.BlockNumber.Uint64(),
		)
		if err != nil {
			e.logger.Error("could not get exit events for checkpoint",
				"err", err,
				"epoch", checkpointSubmittedEvent.Epoch.Uint64(),
				"checkpointBlockNumber", checkpointSubmittedEvent.BlockNumber.Uint64(),
			)

			return err
		}

		if len(exitEvents) == 0 {
			e.logger.Debug("There are no exit events that happened in given given checkpoint")

			return nil
		}

		newEvents := make([]*RelayerEventMetaData, len(exitEvents))
		for i, event := range exitEvents {
			newEvents[i] = &RelayerEventMetaData{EventID: event.ID.Uint64()}
		}

		e.logger.Debug("There are exit events that happened in given given checkpoint", "exitEvents", len(newEvents))

		return e.state.UpdateRelayerEvents(newEvents, nil, nil)
	case exitProcessedEventSig:
		_, err := exitProcessedEvent.ParseLog(eventLog)
		if err != nil {
			e.logger.Error("could not decode exit processed event", "err", err)

			return err
		}

		e.logger.Debug(
			"exit processed event arrived",
			"exitEventID", exitProcessedEvent.ID,
			"success", exitProcessedEvent.Success,
		)

		eventID := exitProcessedEvent.ID.Uint64()

		if exitProcessedEvent.Success {
			e.logger.Debug("exit processed event has been handled", "eventID", eventID)

			return e.state.UpdateRelayerEvents(nil, []uint64{eventID}, nil)
		}

		e.logger.Debug("exit event was not successfully executed",
			"eventID", eventID, "reason", string(exitProcessedEvent.ReturnData))

		return nil
	default:
		return errUnknownExitEvent
	}
}

// sendTx sends unexecuted exit events in a batch to ExitHelper contract
func (e *exitRelayer) sendTx(events []*RelayerEventMetaData) error {
	e.logger.Debug("sending exit events in batch to be executed on ExitHelper", "exitEvents", len(events))
	defer e.logger.Debug("sending exit events in batch to be executed on ExitHelper finished", "exitEvents", len(events))

	inputs := make([]*contractsapi.BatchExitInput, len(events))

	for i, event := range events {
		proof, err := e.proofRetriever.GenerateExitProof(event.EventID)
		if err != nil {
			return fmt.Errorf("failed to get proof for exit event %d: %w", event.EventID, err)
		}

		exitInput, err := GetExitInputFromProof(proof)
		if err != nil {
			return err
		}

		inputs[i] = exitInput
	}

	input, err := (&contractsapi.BatchExitExitHelperFn{
		Inputs: inputs,
	}).EncodeAbi()
	if err != nil {
		return err
	}

	exitTxn := types.NewTx(types.NewLegacyTx(
		types.WithFrom(e.key.Address()),
		types.WithTo(&e.config.eventExecutionAddr),
		types.WithInput(input),
	))

	// send batchExecute exit events
	_, err = e.txRelayer.SendTransaction(exitTxn, e.key)

	return err
}

// GetExitInputFromProof generates exit input from provided exit proof
func GetExitInputFromProof(proof types.Proof) (*contractsapi.BatchExitInput, error) {
	var (
		leafIndexBig       *big.Int
		checkpointBlockBig *big.Int
	)

	leafIndexFloat, ok := proof.Metadata["LeafIndex"].(float64)
	if !ok {
		// try converting into uint64
		leafIndexUInt64, ok := proof.Metadata["LeafIndex"].(uint64)
		if !ok {
			return nil, errors.New("failed to convert proof leaf index")
		}

		leafIndexBig = new(big.Int).SetUint64(leafIndexUInt64)
	} else {
		leafIndexBig = new(big.Int).SetUint64(uint64(leafIndexFloat))
	}

	checkpointBlockFloat, ok := proof.Metadata["CheckpointBlock"].(float64)
	if !ok {
		// try converting into big.Int
		checkpointBlockBig, ok = proof.Metadata["CheckpointBlock"].(*big.Int)
		if !ok {
			return nil, errors.New("failed to convert proof checkpoint block")
		}
	} else {
		checkpointBlockBig = new(big.Int).SetUint64(uint64(checkpointBlockFloat))
	}

	exitEventHex, ok := proof.Metadata["ExitEvent"].(string)
	if !ok {
		return nil, errors.New("failed to convert exit event")
	}

	exitEventEncoded, err := hex.DecodeString(exitEventHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode hex-encoded exit event '%s': %w", exitEventHex, err)
	}

	return &contractsapi.BatchExitInput{
		BlockNumber:  checkpointBlockBig,
		UnhashedLeaf: exitEventEncoded,
		LeafIndex:    leafIndexBig,
		Proof:        proof.Data,
	}, nil
}
