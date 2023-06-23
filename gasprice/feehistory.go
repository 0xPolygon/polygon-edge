package gasprice

import (
	"errors"
	"math/big"
	"sort"
)

var (
	ErrInvalidPercentile = errors.New("invalid percentile")
	ErrBlockCount        = errors.New("blockCount must be greater than 0")
	ErrBlockInfo         = errors.New("could not find block info")
)

const (
	maxBlockRequest = 1024
)

type txGasAndReward struct {
	gasUsed *big.Int
	reward  *big.Int
}

func (g *GasHelper) FeeHistory(blockCount uint64, newestBlock uint64, rewardPercentiles []float64) (*uint64, *[]uint64, *[]float64, *[][]uint64, error) {
	if blockCount < 1 {
		return nil, nil, nil, nil, ErrBlockCount
	}
	if newestBlock > g.backend.Header().Number {
		newestBlock = g.backend.Header().Number
	}
	if blockCount > maxBlockRequest {
		blockCount = maxBlockRequest
	}
	if blockCount > newestBlock {
		blockCount = newestBlock
	}
	for i, p := range rewardPercentiles {
		if p < 0 || p > 100 {
			return nil, nil, nil, nil, ErrInvalidPercentile
		}
		if i > 0 && p < rewardPercentiles[i-1] {
			return nil, nil, nil, nil, ErrInvalidPercentile
		}
	}

	var (
		oldestBlock   = newestBlock - blockCount + 1
		baseFeePerGas = make([]uint64, blockCount+1)
		gasUsedRatio  = make([]float64, blockCount)
		reward        = make([][]uint64, blockCount)
	)
	if oldestBlock < 1 {
		oldestBlock = 1
	}

	for i := oldestBlock; i < oldestBlock+blockCount; i++ {
		block, ok := g.backend.GetBlockByNumber(i, false)
		if !ok {
			return nil, nil, nil, nil, ErrBlockInfo
		}
		baseFeePerGas[i-oldestBlock] = block.Header.BaseFee
		gasUsedRatio[i-oldestBlock] = float64(block.Header.GasUsed) / float64(block.Header.GasLimit)

		if len(rewardPercentiles) == 0 {
			//reward percentiles not requested, skip rest of this loop
			continue
		}
		reward[i-oldestBlock] = make([]uint64, len(rewardPercentiles))
		if len(block.Transactions) == 0 {
			for j := range reward[i-oldestBlock] {
				reward[i-oldestBlock][j] = 0
			}
			//no transactions in block, set rewards to 0 and move to next block
			continue
		}

		sorter := make([]*txGasAndReward, len(block.Transactions))
		for j, tx := range block.Transactions {
			cost := tx.Cost()
			sorter[j] = &txGasAndReward{
				gasUsed: cost.Sub(cost, tx.Value),
				reward:  tx.EffectiveTip(block.Header.BaseFee),
			}
		}
		sort.Slice(sorter, func(i, j int) bool {
			return sorter[i].reward.Cmp(sorter[j].reward) < 0
		})

		var txIndex int
		sumGasUsed := sorter[0].gasUsed.Uint64()

		// calculate reward for each percentile
		for c, v := range rewardPercentiles {
			thresholdGasUsed := uint64(float64(block.Header.GasUsed) * v / 100)
			for sumGasUsed < thresholdGasUsed && txIndex < len(block.Transactions)-1 {
				txIndex++
				sumGasUsed += sorter[txIndex].gasUsed.Uint64()
			}
			reward[i-oldestBlock][c] = sorter[txIndex].reward.Uint64()
		}
	}

	baseFeePerGas[blockCount] = g.backend.Header().BaseFee
	return &oldestBlock, &baseFeePerGas, &gasUsedRatio, &reward, nil
}
