package statesyncrelayer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path"
	"strings"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/tracker"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"

	hcf "github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
)

type StateSyncRelayer struct {
	dataDir                string
	rpcEndpoint            string
	stateReceiverAddr      ethgo.Address
	eventTrackerStartBlock uint64
	logger                 hcf.Logger
	client                 *jsonrpc.Client
	txRelayer              txrelayer.TxRelayer
	key                    ethgo.Key
	closeCh                chan struct{}
}

func sanitizeRPCEndpoint(rpcEndpoint string) string {
	if rpcEndpoint == "" || strings.Contains(rpcEndpoint, "0.0.0.0") {
		_, port, err := net.SplitHostPort(rpcEndpoint)
		if err == nil {
			rpcEndpoint = fmt.Sprintf("http://%s:%s", "127.0.0.1", port)
		} else {
			rpcEndpoint = txrelayer.DefaultRPCAddress
		}
	}

	return rpcEndpoint
}

func NewRelayer(
	dataDir string,
	rpcEndpoint string,
	stateReceiverAddr ethgo.Address,
	stateReceiverTrackerStartBlock uint64,
	logger hcf.Logger,
	key ethgo.Key,
) *StateSyncRelayer {
	endpoint := sanitizeRPCEndpoint(rpcEndpoint)

	// create the JSON RPC client
	client, err := jsonrpc.NewClient(endpoint)
	if err != nil {
		logger.Error("Failed to create the JSON RPC client", "err", err)

		return nil
	}

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(client))
	if err != nil {
		logger.Error("Failed to create the tx relayer", "err", err)
	}

	return &StateSyncRelayer{
		dataDir:                dataDir,
		rpcEndpoint:            endpoint,
		stateReceiverAddr:      stateReceiverAddr,
		logger:                 logger,
		client:                 client,
		txRelayer:              txRelayer,
		key:                    key,
		closeCh:                make(chan struct{}),
		eventTrackerStartBlock: stateReceiverTrackerStartBlock,
	}
}

func (r *StateSyncRelayer) Start() error {
	et := tracker.NewEventTracker(
		path.Join(r.dataDir, "/relayer.db"),
		r.rpcEndpoint,
		r.stateReceiverAddr,
		r,
		0, // sidechain (Polygon POS) is instant finality, so no need to wait
		r.eventTrackerStartBlock,
		r.logger,
	)

	ctx, cancelFn := context.WithCancel(context.Background())

	go func() {
		<-r.closeCh
		cancelFn()
	}()

	return et.Start(ctx)
}

// Stop function is used to tear down all the allocated resources
func (r *StateSyncRelayer) Stop() {
	close(r.closeCh)
}

func (r *StateSyncRelayer) AddLog(log *ethgo.Log) {
	r.logger.Debug("Received a log", "log", log)

	var commitEvent contractsapi.NewCommitmentEvent

	doesMatch, err := commitEvent.ParseLog(log)
	if !doesMatch {
		return
	}

	if err != nil {
		r.logger.Error("Failed to parse log", "err", err)

		return
	}

	startID := commitEvent.StartID.Uint64()
	endID := commitEvent.EndID.Uint64()

	r.logger.Info("Execute commitment", "Block", log.BlockNumber, "StartID", startID, "EndID", endID)

	for i := startID; i <= endID; i++ {
		// query the state sync proof
		stateSyncProof, err := r.queryStateSyncProof(fmt.Sprintf("0x%x", i))
		if err != nil {
			r.logger.Error("Failed to query state sync proof", "err", err)

			continue
		}

		if err := r.executeStateSync(stateSyncProof); err != nil {
			r.logger.Error("Failed to execute state sync", "err", err)

			continue
		}

		r.logger.Info("State sync executed", "ID", i)
	}
}

// queryStateSyncProof queries the state sync proof
func (r *StateSyncRelayer) queryStateSyncProof(stateSyncID string) (*types.Proof, error) {
	// retrieve state sync proof
	var stateSyncProof types.Proof

	err := r.client.Call("bridge_getStateSyncProof", &stateSyncProof, stateSyncID)
	if err != nil {
		return nil, err
	}

	r.logger.Debug(fmt.Sprintf("state sync proof: %v", stateSyncProof))

	return &stateSyncProof, nil
}

// executeStateSync executes the state sync
func (r *StateSyncRelayer) executeStateSync(proof *types.Proof) error {
	sseMap, ok := proof.Metadata["StateSync"].(map[string]interface{})
	if !ok {
		return errors.New("could not get state sync event from proof")
	}

	var sse *contractsapi.StateSync

	// since state sync event is a map in the jsonrpc response,
	// to not have custom logic of converting the map to state sync event
	// json encoding is used, since it manages to successfully unmarshal the
	// event from the marshaled map
	raw, err := json.Marshal(sseMap)
	if err != nil {
		return fmt.Errorf("failed to marshal state sync map into JSON. Error: %w", err)
	}

	if err = json.Unmarshal(raw, &sse); err != nil {
		return fmt.Errorf("failed to unmarshal state sync event from JSON. Error: %w", err)
	}

	execute := &contractsapi.ExecuteStateReceiverFn{
		Proof: proof.Data,
		Obj:   sse,
	}

	input, err := execute.EncodeAbi()
	if err != nil {
		return err
	}

	// execute the state sync
	txn := &ethgo.Transaction{
		From:     r.key.Address(),
		To:       (*ethgo.Address)(&contracts.StateReceiverContract),
		GasPrice: 0,
		Gas:      types.StateTransactionGasLimit,
		Input:    input,
	}

	receipt, err := r.txRelayer.SendTransaction(txn, r.key)
	if err != nil {
		return fmt.Errorf("failed to send state sync transaction: %w", err)
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		return fmt.Errorf("state sync execution failed: %d", execute.Obj.ID)
	}

	return nil
}
