package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
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
	pullTime     time.Duration
	receiptDelay time.Duration
}

func NewAARelayerService(
	txSender AATxSender,
	pool AAPool,
	state AATxState,
	key ethgo.Key,
	invokerAddr types.Address,
	opts ...TxRelayerOption) *AARelayerService {
	service := &AARelayerService{
		txSender:     txSender,
		pool:         pool,
		state:        state,
		key:          key,
		invokerAddr:  invokerAddr,
		pullTime:     time.Millisecond * 5000, // every five seconds pull from pool
		receiptDelay: time.Millisecond * 50,
	}

	for _, opt := range opts {
		opt(service)
	}

	return service
}

func (rs *AARelayerService) Start(ctx context.Context) {
	ticker := time.NewTicker(rs.pullTime)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stateTx := rs.pool.Pop()
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

	stateTx.Status = StatusQueued
	if err := rs.state.Update(stateTx); err != nil {
		// TODO: log error but do not return
		fmt.Printf("error while updating state tx = %s after sending it, err = %v", stateTx.ID, err)
	}

	select {
	case <-ctx.Done():
		return nil
	default:
	}

	receipt, err := rs.txSender.WaitForReceipt(ctx, hash, rs.receiptDelay)
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
	signature, tx := stateTx.Tx.ToAbi()
	params := &contractsapi.InvokeFunction{
		Signature:   signature,
		Transaction: tx,
	}

	input, err := params.EncodeAbi()
	if err != nil {
		return nil, err
	}

	return &ethgo.Transaction{
		From:  rs.key.Address(),
		To:    (*ethgo.Address)(&rs.invokerAddr),
		Input: input,
	}, nil
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
