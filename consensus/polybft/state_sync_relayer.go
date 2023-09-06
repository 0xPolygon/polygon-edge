package polybft

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/common"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	hclog "github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
)

var errFailedToExecuteStateSync = errors.New("failed to execute state sync")

type StateSyncRelayer interface {
	PostBlock(req *common.PostBlockRequest) error
	Init() error
	Close()
}

type stateSyncProofRetriever interface {
	GetStateSyncProof(stateSyncID uint64) (types.Proof, error)
}

var _ StateSyncRelayer = (*dummyStakeSyncRelayer)(nil)

// dummyStakeSyncRelayer is a dummy implementation of a StateSyncRelayer used only for unit testing
type dummyStakeSyncRelayer struct{}

func (d *dummyStakeSyncRelayer) PostBlock(req *common.PostBlockRequest) error { return nil }

func (d *dummyStakeSyncRelayer) Init() error { return nil }
func (d *dummyStakeSyncRelayer) Close()      {}

var _ StateSyncRelayer = (*stateSyncRelayerImpl)(nil)

type stateSyncRelayerImpl struct {
	txRelayer    txrelayer.TxRelayer
	key          ethgo.Key
	store        stateSyncProofRetriever
	state        *State
	eventsGetter *eventsGetter[*contractsapi.NewCommitmentEvent]
	logger       hclog.Logger

	dataCh  chan *common.PostBlockRequest
	closeCh chan struct{}
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
		txRelayer: txRelayer,
		key:       key,
		store:     store,
		state:     state,
		closeCh:   make(chan struct{}),
		dataCh:    make(chan *common.PostBlockRequest, 32),
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
			case req := <-ssr.dataCh:
				stateNextBlockNumber, stateNextEventID, err := ssr.state.StateSyncStore.getStateSyncRelayerData()
				if err != nil {
					ssr.logger.Error("state sync relayer get state failed", "err", err)

					break
				}

				nextBlockNum, nextEventID, err := ssr.handleNewBlock(
					req.FullBlock.Block.Header, req.FullBlock.Receipts, stateNextBlockNumber, stateNextEventID)
				if err != nil {
					ssr.logger.Error("state sync relayer failed",
						"state block", stateNextBlockNumber, "state eventID", stateNextEventID,
						"block", nextBlockNum, "eventID", nextEventID,
						"err", err)
				}

				// we should write data back to database even if handleNewBlock returned some error
				if err = ssr.state.StateSyncStore.insertStateSyncRelayerData(
					nextBlockNum, nextEventID); err != nil {
					ssr.logger.Error("state sync relayer insert state failed",
						"state block", stateNextBlockNumber, "state eventID", stateNextEventID,
						"block", nextBlockNum, "eventID", nextEventID,
						"err", err)
				}
			}
		}
	}()

	return nil
}

func (ssr *stateSyncRelayerImpl) Close() {
	close(ssr.closeCh)
}

func (ssr *stateSyncRelayerImpl) PostBlock(req *common.PostBlockRequest) error {
	ssr.dataCh <- req // add to consumer queue

	return nil
}

func (ssr *stateSyncRelayerImpl) handleNewBlock(
	header *types.Header, receipts []*types.Receipt,
	nextBlockNumber uint64, nextEventID uint64) (uint64, uint64, error) {
	var (
		err    error
		events []*contractsapi.NewCommitmentEvent
	)

	for nextBlockNumber < header.Number {
		if events, err = ssr.eventsGetter.getEvents(nextBlockNumber); err != nil {
			return nextBlockNumber, nextEventID, err
		}

		if nextEventID, err = ssr.handleEvents(nextEventID, events); err != nil {
			return nextBlockNumber, nextEventID, err
		}

		nextBlockNumber++
	}

	currentEvents, err := ssr.eventsGetter.getEventsFromReceipts(header, receipts)
	if err != nil {
		return nextBlockNumber, nextEventID, err
	}

	if nextEventID, err = ssr.handleEvents(nextEventID, currentEvents); err != nil {
		return nextBlockNumber, nextEventID, err
	}

	return header.Number + 1, nextEventID, nil
}

func (ssr *stateSyncRelayerImpl) handleEvents(
	nextEventID uint64, events []*contractsapi.NewCommitmentEvent) (uint64, error) {
	for _, event := range events {
		if nextEventID > event.EndID.Uint64() {
			continue // this event is already processed, but maybe not all events in block are
		}

		if nextEventID < event.StartID.Uint64() {
			nextEventID = event.StartID.Uint64() // this condition should be executed only first time
		}

		for nextEventID <= event.EndID.Uint64() {
			ssr.logger.Info("execute commitment", "id", nextEventID)

			proof, err := ssr.store.GetStateSyncProof(nextEventID)
			if err != nil {
				// ssr.logger.Error("failed to query proof", "id", currEventID, "err", err)
				return nextEventID, fmt.Errorf("failed to query proof: %w", err)
			}

			tx, err := ssr.createTx(proof)
			if err != nil {
				// ssr.logger.Error("failed to create tx", "id", currEventID, "err", err)
				return nextEventID, fmt.Errorf("failed to create tx: %w", err)
			}

			if err := ssr.sendTx(tx); err != nil {
				return nextEventID, fmt.Errorf("failed to execute tx: %w", err)
			}

			nextEventID++ // update nextEventID
		}
	}

	return nextEventID, nil
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
