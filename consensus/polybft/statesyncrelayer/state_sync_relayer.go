package statesyncrelayer

import (
	"fmt"
	"math/big"
	"net"
	"path"
	"strconv"
	"strings"

	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/tracker"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"

	hcf "github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/jsonrpc"
)

var commitEvent = abi.MustNewEvent(`event NewCommitment(uint256 startId, uint256 endId, bytes32 root)`)

type StateSyncRelayer struct {
	dataDir           string
	rpcEndpoint       string
	stateReceiverAddr ethgo.Address
	logger            hcf.Logger
	client            *jsonrpc.Client
	txRelayer         txrelayer.TxRelayer
	key               ethgo.Key
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

	return et.Start()
}

func (r *StateSyncRelayer) AddLog(log *ethgo.Log) {
	r.logger.Info("Received a log", "log", log)

	if commitEvent.Match(log) {
		vals, err := commitEvent.ParseLog(log)
		if err != nil {
			panic(err)
		}

		var startID uint64
		if sid, ok := vals["startId"].(big.Int); ok {
			startID = sid.Uint64()
		}

		var endID uint64
		if eid, ok := vals["endId"].(big.Int); ok {
			endID = eid.Uint64()
		}

		r.logger.Info("Commit", "Block", log.BlockNumber, "StartID", startID, "EndID", endID)

		for i := startID; i <= endID; i++ {
			// query the state sync proof
			stateSyncProof, err := r.queryStateSyncProof(strconv.Itoa(int(i)))
			if err != nil {
				r.logger.Error("Failed to query state sync proof", "err", err)

				continue
			}

			if err := r.executeStateSync(stateSyncProof); err != nil {
				r.logger.Error("Failed to execute state sync", "err", err)
			}
		}
	}
}

// queryStateSyncProof queries the state sync proof
func (r *StateSyncRelayer) queryStateSyncProof(stateSyncID string) (*types.StateSyncProof, error) {
	// retrieve state sync proof
	var stateSyncProof types.StateSyncProof

	err := r.client.Call("bridge_getStateSyncProof", &stateSyncProof, stateSyncID)
	if err != nil {
		return nil, err
	}

	r.logger.Debug(fmt.Sprintf("state sync proof: %v", stateSyncProof))

	return &stateSyncProof, nil
}

// executeStateSync executes the state sync
func (r *StateSyncRelayer) executeStateSync(stateSyncProof *types.StateSyncProof) error {
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

	if receipt.Status == uint64(types.ReceiptSuccess) {
		return fmt.Errorf("state sync execution failed")
	}

	return nil
}
