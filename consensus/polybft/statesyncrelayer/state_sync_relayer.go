package statesyncrelayer

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net"
	"path"
	"strings"

	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/tracker"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"

	hcf "github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
)

var commitEvent = contractsapi.StateReceiver.Abi.Events["NewCommitment"]

type StateSyncRelayer struct {
	dataDir           string
	rpcEndpoint       string
	stateReceiverAddr ethgo.Address
	logger            hcf.Logger
	client            *jsonrpc.Client
	txRelayer         txrelayer.TxRelayer
	key               ethgo.Key
	closeCh           chan struct{}
}

func sanitizeRPCEndpoint(rpcEndpoint string) string {
	if rpcEndpoint == "" || strings.Contains(rpcEndpoint, "0.0.0.0") {
		_, port, err := net.SplitHostPort(rpcEndpoint)
		if err == nil {
			rpcEndpoint = fmt.Sprintf("http://%s:%s", "127.0.0.1", port)
		} else {
			rpcEndpoint = "http://127.0.0.1:8545"
		}
	}

	return rpcEndpoint
}

func NewRelayer(
	dataDir string,
	rpcEndpoint string,
	stateReceiverAddr ethgo.Address,
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
		dataDir:           dataDir,
		rpcEndpoint:       endpoint,
		stateReceiverAddr: stateReceiverAddr,
		logger:            logger,
		client:            client,
		txRelayer:         txRelayer,
		key:               key,
		closeCh:           make(chan struct{}),
	}
}

func (r *StateSyncRelayer) Start() error {
	et := tracker.NewEventTracker(
		path.Join(r.dataDir, "/relayer.db"),
		r.rpcEndpoint,
		r.stateReceiverAddr,
		r,
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
	r.logger.Info("Received a log", "log", log)

	if commitEvent.Match(log) {
		vals, err := commitEvent.ParseLog(log)
		if err != nil {
			r.logger.Error("Failed to parse log", "err", err)

			return
		}

		var (
			startID, endID *big.Int
			ok             bool
		)

		if startID, ok = vals["startId"].(*big.Int); !ok {
			r.logger.Error("Failed to parse startId")

			return
		}

		if endID, ok = vals["endId"].(*big.Int); !ok {
			r.logger.Error("Failed to parse endId")

			return
		}

		r.logger.Info("Commit", "Block", log.BlockNumber, "StartID", startID, "EndID", endID)

		for i := startID.Int64(); i <= endID.Int64(); i++ {
			// query the state sync proof
			stateSyncProof, err := r.queryStateSyncProof(fmt.Sprintf("0x%x", int(i)))
			if err != nil {
				r.logger.Error("Failed to query state sync proof", "err", err)

				continue
			}

			if err := r.executeStateSync(stateSyncProof); err != nil {
				r.logger.Error("Failed to execute state sync", "err", err)

				continue
			}

			r.logger.Info("State sync executed", "stateSyncID", i)
		}
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
	sse, ok := proof.Metadata["StateSync"].(*contractsapi.StateSyncedEvent)
	if !ok {
		return errors.New("could not get state sync event from proof")
	}

	stateSyncProof := &polybft.StateSyncProof{
		Proof:     proof.Data,
		StateSync: sse,
	}

	input, err := stateSyncProof.EncodeAbi()
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
		return fmt.Errorf("state sync execution failed: %d", stateSyncProof.StateSync.ID)
	}

	return nil
}
