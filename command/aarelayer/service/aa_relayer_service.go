package service

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"
)

const (
	receiptSuccess  = 1
	defaultGasLimit = 5242880 // 0x500000
)

// AARelayerService pulls transaction from pool one at the time and sends it to relayer
type AARelayerService struct {
	pool         AAPool
	state        AATxState
	rpcClient    AARPCClient
	key          ethgo.Key
	invokerAddr  types.Address
	chainID      int64
	currentNonce uint64
	pullTime     time.Duration // pull from txpool every `pullTime` second/millisecond
	receiptDelay time.Duration
	numRetries   int
	logger       hclog.Logger
}

func NewAARelayerService(
	rpcClient AARPCClient,
	pool AAPool,
	state AATxState,
	key ethgo.Key,
	invokerAddr types.Address,
	chainID int64,
	logger hclog.Logger,
	opts ...TxRelayerOption) (*AARelayerService, error) {
	nonce, err := rpcClient.GetNonce(key.Address()) // get initial nonce for the aarelayer
	if err != nil {
		return nil, err
	}

	service := &AARelayerService{
		rpcClient:    rpcClient,
		pool:         pool,
		state:        state,
		key:          key,
		invokerAddr:  invokerAddr,
		chainID:      chainID,
		currentNonce: nonce,
		pullTime:     time.Millisecond * 5000,
		receiptDelay: time.Millisecond * 500,
		numRetries:   100,
		logger:       logger.Named("service"),
	}

	for _, opt := range opts {
		opt(service)
	}

	return service, nil
}

func (rs *AARelayerService) Start(ctx context.Context) {
	ticker := time.NewTicker(rs.pullTime)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stateTx := rs.getFirstValidTx()
			if stateTx == nil { // nothing to process
				continue
			}

			if err := rs.addToQueue(stateTx); err != nil {
				rs.logger.Error(
					"error while adding transaction to the queue",
					"id", stateTx.ID,
					"from", stateTx.Tx.Transaction.From,
					"err", err)

				continue
			}

			go func() {
				if err := rs.executeJob(ctx, stateTx); err != nil {
					rs.logger.Error(
						"transaction execution has been failed",
						"id", stateTx.ID,
						"from", stateTx.Tx.Transaction.From,
						"nonce", stateTx.Nonce,
						"err", err)
				}
			}()
		}
	}
}

func (rs *AARelayerService) executeJob(ctx context.Context, stateTx *AAStateTransaction) error {
	if err := rs.sendTransaction(stateTx); err != nil {
		return err
	}

	return rs.waitForReceipt(ctx, stateTx)
}

func (rs *AARelayerService) makeEthgoTransaction(stateTx *AAStateTransaction) ([]byte, ethgo.Hash, error) {
	gasPrice, err := rs.rpcClient.GetGasPrice()
	if err != nil {
		return nil, ethgo.ZeroHash, err
	}

	input, err := stateTx.Tx.ToAbi()
	if err != nil {
		return nil, ethgo.ZeroHash, err
	}

	tx := &ethgo.Transaction{
		From:     rs.key.Address(),
		To:       (*ethgo.Address)(&rs.invokerAddr),
		Input:    input,
		Nonce:    stateTx.Nonce,
		Gas:      defaultGasLimit,
		ChainID:  big.NewInt(rs.chainID),
		GasPrice: gasPrice,
	}

	signer := wallet.NewEIP155Signer(tx.ChainID.Uint64())
	if _, err := signer.SignTx(tx, rs.key); err != nil {
		return nil, ethgo.ZeroHash, err
	}

	hash, err := tx.GetHash()
	if err != nil {
		return nil, ethgo.ZeroHash, err
	}

	raw, err := tx.MarshalRLPTo(nil)
	if err != nil {
		return nil, ethgo.ZeroHash, err
	}

	return raw, hash, nil
}

// getFirstValidTx takes from the pool first arrived transaction which nonce is good
func (rs *AARelayerService) getFirstValidTx() *AAStateTransaction {
	var (
		pushBackList []*AAStateTransaction
		stateTx      = (*AAStateTransaction)(nil)
	)

	for {
		poppedTx := rs.pool.Pop()
		if poppedTx == nil {
			break
		}

		address := poppedTx.Tx.Transaction.From

		nonce, err := rs.rpcClient.GetAANonce(ethgo.Address(rs.invokerAddr), ethgo.Address(address))
		if err != nil {
			rs.logger.Warn("transaction retrieving nonce failed",
				"tx", poppedTx.ID, "from", address, "err", err)

			pushBackList = append(pushBackList, poppedTx)

			break
		}

		if nonce == poppedTx.Tx.Transaction.Nonce {
			stateTx = poppedTx
			// update pool -> put statetx with next nonce to the timeHeap
			rs.pool.Update(stateTx.Tx.Transaction.From)

			break
		} else if poppedTx.Tx.Transaction.Nonce > nonce {
			// if tx nonce is greater current nonce than the tx will be returned back to the pool for later processing
			rs.logger.Debug("transaction nonce mismatch",
				"tx", poppedTx.ID, "from", address,
				"nonce", poppedTx.Tx.Transaction.Nonce, "expected", nonce)

			pushBackList = append(pushBackList, poppedTx)
		} else {
			// ...otherwise if it is lower update status to failed and never process it again
			errorStr := "nonce too low"
			poppedTx.Error = &errorStr
			poppedTx.Status = StatusFailed

			if err := rs.state.Update(stateTx); err != nil {
				rs.logger.Debug("fail to update transaction status, nonce too low",
					"tx", poppedTx.ID, "from", address, "nonce", poppedTx.Tx.Transaction.Nonce, "err", err)
			} else {
				rs.logger.Info("transaction status has been changed to failed because of low nonce",
					"tx", poppedTx.ID, "from", address, "nonce", poppedTx.Tx.Transaction.Nonce)
			}
		}
	}

	// return all transactions with incorrect nonces to the list
	for _, x := range pushBackList {
		rs.pool.Push(x)
	}

	return stateTx
}

func (rs *AARelayerService) addToQueue(stateTx *AAStateTransaction) error {
	stateTx.Nonce = atomic.LoadUint64(&rs.currentNonce) // setup nonce for this transaction

	txRaw, hash, err := rs.makeEthgoTransaction(stateTx)
	if err != nil {
		rs.pool.Push(stateTx) // if something went wrong in this step,  tx should return to the pending pool

		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// transaction is now in the queued state
	stateTx.Status = StatusQueued
	stateTx.Hash = hash
	stateTx.Raw = txRaw

	if err := rs.state.Update(stateTx); err != nil {
		rs.pool.Push(stateTx) // if update status fails, tx should return to the pending pool

		return fmt.Errorf("failed to update transaction state to queued: %w", err)
	}

	atomic.AddUint64(&rs.currentNonce, 1) // increment current aa relayer nonce

	return nil
}

func (rs *AARelayerService) sendTransaction(stateTx *AAStateTransaction) error {
	var netErr net.Error

	_, err := rs.rpcClient.SendTransaction(stateTx.Raw)
	if errors.As(err, &netErr) {
		return fmt.Errorf("failed to send transaction: %w", err)
	} else if err != nil {
		errstr := err.Error()
		stateTx.Error = &errstr
		stateTx.Status = StatusFailed // change status on all non network related errors?

		if errUpdate := rs.state.Update(stateTx); errUpdate != nil {
			return fmt.Errorf("failed to send transaction: %w, update error: %s", err, errUpdate.Error())
		}

		return err
	}

	rs.logger.Info("transaction has been sent to the invoker", "id", stateTx.ID)

	// transaction is now in the sent state
	stateTx.Status = StatusSent
	if err := rs.state.Update(stateTx); err != nil {
		// if fails, just log error. We should figure out how to update to status sent if state update fails
		rs.logger.Warn("failed to update transaction state to sent", "id", stateTx.ID, "err", err)
	}

	return nil
}

func (rs *AARelayerService) waitForReceipt(ctx context.Context, stateTx *AAStateTransaction) error {
	receipt, err := rs.rpcClient.WaitForReceipt(ctx, stateTx.Hash, rs.receiptDelay, rs.numRetries)
	if err != nil {
		errstr := err.Error()
		stateTx.Error = &errstr
		stateTx.Status = StatusFailed

		rs.logger.Warn("transaction receipt error",
			"id", stateTx.ID, "hash", stateTx.Hash, "err", err)
	} else {
		rs.populateStateTx(stateTx, receipt)
	}

	if err := rs.state.Update(stateTx); err != nil {
		return fmt.Errorf("failed to update transaction state to %s, err = %w", stateTx.Status, err)
	}

	return nil
}

func (rs *AARelayerService) populateStateTx(stateTx *AAStateTransaction, receipt *ethgo.Receipt) {
	stateTx.Gas = receipt.GasUsed
	stateTx.Mined = &Mined{
		BlockHash:   types.Hash(receipt.BlockHash),
		BlockNumber: receipt.BlockNumber,
		TxnHash:     types.Hash(receipt.TransactionHash),
		GasUsed:     receipt.GasUsed,
		Logs:        make([]Log, len(receipt.Logs)),
	}

	for i, log := range receipt.Logs {
		topics := make([]types.Hash, len(log.Topics))

		for j, topic := range log.Topics {
			topics[j] = types.Hash(topic)
		}

		stateTx.Mined.Logs[i] = Log{
			Address: types.Address(log.Address),
			Data:    log.Data,
			Topics:  topics,
		}
	}

	if receipt.Status == receiptSuccess {
		stateTx.Status = StatusCompleted

		rs.logger.Debug("transaction has been executed successfully",
			"id", stateTx.ID,
			"from", stateTx.Tx.Transaction.From,
			"nonce", stateTx.Tx.Transaction.Nonce)
	} else {
		stateTx.Status = StatusFailed

		rs.logger.Warn("transaction receipt status failed",
			"id", stateTx.ID,
			"from", stateTx.Tx.Transaction.From,
			"nonce", stateTx.Tx.Transaction.Nonce)
	}
}

type TxRelayerOption func(*AARelayerService)

func WithPullTime(pullTime time.Duration) TxRelayerOption {
	return func(t *AARelayerService) {
		t.pullTime = pullTime
	}
}

func WithReceiptDelay(receiptDelay time.Duration) TxRelayerOption {
	return func(t *AARelayerService) {
		t.receiptDelay = receiptDelay
	}
}

func WithNumRetries(numRetries int) TxRelayerOption {
	return func(t *AARelayerService) {
		t.numRetries = numRetries
	}
}
