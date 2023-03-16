package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
)

// AARelayerService pulls transaction from pool one at the time and sends it to relayer
type AARelayerService struct {
	pool         AAPool
	state        AATxState
	txSender     AATxSender
	key          ethgo.Key
	invokerAddr  types.Address
	currentNonce uint64
	pullTime     time.Duration // pull from txpool every `pullTime` second/millisecond
	receiptDelay time.Duration
	numRetries   int
}

func NewAARelayerService(
	txSender AATxSender,
	pool AAPool,
	state AATxState,
	key ethgo.Key,
	invokerAddr types.Address,
	opts ...TxRelayerOption) (*AARelayerService, error) {
	nonce, err := txSender.GetNonce(key.Address())
	if err != nil {
		return nil, err
	}

	service := &AARelayerService{
		txSender:     txSender,
		pool:         pool,
		state:        state,
		key:          key,
		invokerAddr:  invokerAddr,
		currentNonce: nonce,
		pullTime:     time.Millisecond * 5000,
		receiptDelay: time.Millisecond * 500,
		numRetries:   100,
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

			if stateTx != nil { // there is something to process
				go func() {
					if err := rs.executeJob(ctx, stateTx); err != nil {
						// TODO: log error in file not just fmt.Println
						fmt.Println(err)
					}
				}()
			}
		}
	}
}

func (rs *AARelayerService) executeJob(ctx context.Context, stateTx *AAStateTransaction) error {
	var netErr net.Error

	fmt.Printf("executing transaction (%s, %s) with nonce = %d\n",
		stateTx.ID, stateTx.Tx.Transaction.From.String(), stateTx.Tx.Transaction.Nonce)

	tx, err := rs.makeEthgoTransaction(stateTx)
	if err != nil {
		// this should not happened
		rs.pool.Push(stateTx)

		return err
	}

	hash, err := rs.txSender.SendTransaction(tx, rs.key)
	// if its network error return tx back to the pool
	if errors.As(err, &netErr) {
		rs.pool.Push(stateTx)

		return err
	} else if err != nil {
		errstr := err.Error()
		stateTx.Error = &errstr

		if errUpdate := rs.state.Update(stateTx); errUpdate != nil {
			errstr = errUpdate.Error()

			return fmt.Errorf("error while getting nonce for state tx = %s, err = %w, update error = %s",
				stateTx.ID, err, errstr)
		}

		return fmt.Errorf("error while getting nonce for state tx = %s, err = %w", stateTx.ID, err)
	}

	atomic.AddUint64(&rs.currentNonce, 1) // increment global nonce for this relayer

	stateTx.Status = StatusQueued
	if err := rs.state.Update(stateTx); err != nil {
		// TODO: log error but do not return
		fmt.Printf("error while updating state tx = %s after sending it, err = %v\n", stateTx.ID, err)
	}

	fmt.Printf("transaction (%s, %s) with nonce = %d has been sent to the invoker\n",
		stateTx.ID, stateTx.Tx.Transaction.From.String(), stateTx.Tx.Transaction.Nonce)

	receipt, err := rs.txSender.WaitForReceipt(ctx, hash, rs.receiptDelay, rs.numRetries)
	if err != nil {
		errstr := err.Error()
		stateTx.Error = &errstr
		stateTx.Status = StatusFailed
	} else {
		populateStateTx(stateTx, receipt)
		if receipt.Status == 1 { // Status == 1 is ReceiptSuccess status
			stateTx.Status = StatusCompleted
		} else {
			stateTx.Status = StatusFailed
		}
	}

	if err := rs.state.Update(stateTx); err != nil {
		return fmt.Errorf("error while updating state tx = %s, err = %w", stateTx.ID, err)
	}

	return nil
}

func (rs *AARelayerService) makeEthgoTransaction(stateTx *AAStateTransaction) (*ethgo.Transaction, error) {
	// TODO: encode stateTx to input
	return &ethgo.Transaction{
		From:  rs.key.Address(),
		Input: nil,
		Nonce: atomic.LoadUint64(&rs.currentNonce),
	}, nil
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

		nonce, err := rs.txSender.GetAANonce(ethgo.Address(rs.invokerAddr), ethgo.Address(address))
		if err != nil {
			// TODO: log
			fmt.Printf("failed to retrieve nonce for tx (%s, %s): %v", poppedTx.ID, address.String(), err)

			pushBackList = append(pushBackList, poppedTx)

			break
		}

		if nonce != poppedTx.Tx.Transaction.Nonce {
			// TODO: log
			fmt.Printf("transaction can not be sent to invoker - different nonces %d vs %d: %s \n",
				poppedTx.Tx.Transaction.Nonce, nonce, poppedTx.ID)

			pushBackList = append(pushBackList, poppedTx)
		} else {
			stateTx = poppedTx
			// update pool -> put statetx with next nonce to the timeHeap
			rs.pool.Update(stateTx.Tx.Transaction.From)

			break
		}
	}

	// return all transactions with incorrect nonces to the list
	for _, x := range pushBackList {
		rs.pool.Push(x)
	}

	return stateTx
}

func populateStateTx(stateTx *AAStateTransaction, receipt *ethgo.Receipt) {
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
