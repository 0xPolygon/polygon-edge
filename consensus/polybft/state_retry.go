package polybft

import (
	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
)

type eventDBInsertRetry[T contractsapi.EventAbi] struct {
	blockchain   blockchainBackend
	saveEventsFn func([]T) error
	parseEventFn func(*types.Header, *ethgo.Log) (T, bool, error)
	isValidLogFn func(*types.Log) bool
}

func (r *eventDBInsertRetry[T]) insertRetry(fromBlock, toBlock uint64) error {
	events := make([]T, 0)

	for i := fromBlock; i <= toBlock; i++ {
		blockHeader, found := r.blockchain.GetHeaderByNumber(i)
		if !found {
			return blockchain.ErrNoBlock
		}

		receipts, err := r.blockchain.GetReceiptsByHash(blockHeader.Hash)
		if err != nil {
			return err
		}

		for _, receipt := range receipts {
			if receipt.Status == nil || *receipt.Status != types.ReceiptSuccess {
				continue
			}

			for _, log := range receipt.Logs {
				if r.isValidLogFn != nil && !r.isValidLogFn(log) {
					continue
				}

				event, doesMatch, err := r.parseEventFn(blockHeader, convertLog(log))
				if err != nil {
					return err
				}

				if !doesMatch {
					continue
				}

				events = append(events, event)
			}
		}
	}

	if len(events) > 0 {
		return r.saveEventsFn(events)
	}

	return nil
}
