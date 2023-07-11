package gasprice

import (
	"fmt"
	"math/big"
	"sort"
	"sync"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	lru "github.com/hashicorp/golang-lru"
	"github.com/umbracle/ethgo"
)

const couldNotFoundBlockFormat = "could not find block. Number: %d, Hash: %s"

// DefaultGasHelperConfig is the default config for gas helper (as per ethereum)
var DefaultGasHelperConfig = &Config{
	NumOfBlocksToCheck: 20,
	PricePercentile:    60,
	SampleNumber:       3,
	MaxPrice:           ethgo.Gwei(500),
	LastPrice:          ethgo.Gwei(1),
	IgnorePrice:        big.NewInt(2), // 2 wei
}

// Config is a struct that holds configuration of GasHelper
type Config struct {
	// NumOfBlocksToCheck is the number of blocks to sample
	NumOfBlocksToCheck uint64
	// PricePercentile is the sample percentile of transactions in a block
	PricePercentile uint64
	// SampleNumber is number of transactions sampled in a block
	SampleNumber uint64
	// MaxPrice is the tip max price
	MaxPrice *big.Int
	// LastPrice is the last price returned for maxPriorityFeePerGas
	// when starting node it will be some default value
	LastPrice *big.Int
	// IgnorePrice is the lowest price to take into consideration
	// when collecting transactions
	IgnorePrice *big.Int
}

// Blockchain is the interface representing blockchain
type Blockchain interface {
	GetBlockByNumber(number uint64, full bool) (*types.Block, bool)
	GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool)
	Header() *types.Header
	Config() *chain.Params
}

// GasStore interface is providing functions regarding gas and fees
type GasStore interface {
	// MaxPriorityFeePerGas calculates the priority fee needed for transaction to be included in a block
	MaxPriorityFeePerGas() (*big.Int, error)
	// FeeHistory returns the collection of historical gas information
	FeeHistory(uint64, uint64, []float64) (*FeeHistoryReturn, error)
}

var _ GasStore = (*GasHelper)(nil)

// GasHelper struct implements functions from the GasStore interface
type GasHelper struct {
	// numOfBlocksToCheck is the number of blocks to sample
	numOfBlocksToCheck uint64
	// pricePercentile is the sample percentile of transactions in a block
	pricePercentile uint64
	// sampleNumber is number of transactions sampled in a block
	sampleNumber uint64
	// maxPrice is the tip max price
	maxPrice *big.Int
	// lastPrice is the last price returned for maxPriorityFeePerGas
	lastPrice *big.Int
	// ignorePrice is the lowest price to take into consideration
	// when collecting transactions
	ignorePrice *big.Int
	// backend is an abstraction of blockchain
	backend Blockchain
	// lastHeaderHash is the last header for which maxPriorityFeePerGas was returned
	lastHeaderHash types.Hash

	lock sync.Mutex

	historyCache *lru.Cache
}

// NewGasHelper is the constructor function for GasHelper struct
func NewGasHelper(config *Config, backend Blockchain) (*GasHelper, error) {
	pricePercentile := config.PricePercentile
	if pricePercentile > 100 {
		pricePercentile = 100
	}

	cache, err := lru.New(100)
	if err != nil {
		return nil, err
	}

	return &GasHelper{
		numOfBlocksToCheck: config.NumOfBlocksToCheck,
		pricePercentile:    pricePercentile,
		sampleNumber:       config.SampleNumber,
		ignorePrice:        config.IgnorePrice,
		lastPrice:          config.LastPrice,
		maxPrice:           config.MaxPrice,
		backend:            backend,
		historyCache:       cache,
	}, nil
}

// MaxPriorityFeePerGas calculates the priority fee needed for transaction to be included in a block
// The function does following:
//   - takes chain header
//   - iterates for numOfBlocksToCheck from chain header to previous blocks
//   - collects at most the sample number of sorted transactions in block
//   - if not enough transactions were collected and their tips, go through some more blocks to get
//     more accurate calculation
//   - when enough transactions and their tips are collected, take the one that is in pricePercentile
//   - if given price is larger then maxPrice then return the maxPrice
func (g *GasHelper) MaxPriorityFeePerGas() (*big.Int, error) {
	currentHeader := g.backend.Header()

	currentBlock, found := g.backend.GetBlockByHash(currentHeader.Hash, true)
	if !found {
		return nil, fmt.Errorf(couldNotFoundBlockFormat, currentHeader.Number, currentHeader.Hash)
	}

	g.lock.Lock()
	lastPrice := g.lastPrice
	lastHeader := g.lastHeaderHash
	g.lock.Unlock()

	if currentHeader.Hash == lastHeader {
		// small optimization, if we calculated already the price for given block
		return new(big.Int).Set(lastPrice), nil
	}

	var allPrices []*big.Int

	collectPrices := func(block *types.Block) error {
		baseFee := new(big.Int).SetUint64(block.Header.BaseFee)
		txSorter := newTxByEffectiveTipSorter(block.Transactions, baseFee)
		sort.Sort(txSorter)

		blockMiner := types.BytesToAddress(block.Header.Miner)
		signer := crypto.NewSigner(g.backend.Config().Forks.At(block.Number()),
			uint64(g.backend.Config().ChainID))
		blockTxPrices := make([]*big.Int, 0)

		for _, tx := range txSorter.txs {
			tip := tx.EffectiveGasTip(baseFee)

			if tip.Cmp(g.ignorePrice) == -1 {
				// ignore transactions with tip lower than ignore price
				continue
			}

			sender, err := signer.Sender(tx)
			if err != nil {
				return fmt.Errorf("could not get sender of transaction: %s. Error: %w", tx.Hash, err)
			}

			if sender != blockMiner {
				blockTxPrices = append(blockTxPrices, tip)

				// if sample number of txs from block is reached,
				// don't process any more txs
				if len(blockTxPrices) >= int(g.sampleNumber) {
					break
				}
			}
		}

		if len(blockTxPrices) == 0 {
			// either block is empty or all transactions in block are sent by the miner.
			// in this case add the latests calculated price for sampling
			blockTxPrices = append(blockTxPrices, lastPrice)
		}

		// add the block prices to the slice of all prices
		allPrices = append(allPrices, blockTxPrices...)

		return nil
	}

	// iterate from current block to previous blocks determined by numOfBlocksToCheck
	// if chain doesn't have that many blocks, we need to stop the loop (currentBlock.Number() > 0)
	for i := uint64(0); i < g.numOfBlocksToCheck && currentBlock.Number() > 0; i++ {
		if err := collectPrices(currentBlock); err != nil {
			return nil, err
		}

		currentBlock, found = g.backend.GetBlockByHash(currentBlock.ParentHash(), true)
		if !found {
			return nil, fmt.Errorf(couldNotFoundBlockFormat, currentHeader.Number, currentHeader.Hash)
		}
	}

	// at least amount of transactions to get
	minNumOfTx := int(g.numOfBlocksToCheck) * 2
	// collect some more blocks and transactions if not enough transactions were collected
	for len(allPrices) < minNumOfTx && currentBlock.Number() > 0 {
		if err := collectPrices(currentBlock); err != nil {
			return nil, err
		}
	}

	price := lastPrice

	if len(allPrices) > 0 {
		// sort prices from lowest to highest
		sort.Slice(allPrices, func(i, j int) bool {
			return allPrices[i].Cmp(allPrices[j]) < 0
		})
		// take the biggest price that is in the configured percentage
		// by default it's 60, so it will take the price on that percentage
		// of all prices in the array
		price = allPrices[(len(allPrices)-1)*int(g.pricePercentile)/100]
	}

	if price.Cmp(g.maxPrice) > 0 {
		// if price is larger than the configured max price
		// return max price
		price = new(big.Int).Set(g.maxPrice)
	}

	// cache the calculated price and header hash
	g.lock.Lock()
	g.lastPrice = price
	g.lastHeaderHash = currentHeader.Hash
	g.lock.Unlock()

	return price, nil
}

// txSortedByEffectiveTip sorts transactions by effective tip from smallest to largest
type txSortedByEffectiveTip struct {
	txs     []*types.Transaction
	baseFee *big.Int
}

// newTxByEffectiveTipSorter is constructor function for txSortedByEffectiveTip
func newTxByEffectiveTipSorter(txs []*types.Transaction, baseFee *big.Int) *txSortedByEffectiveTip {
	return &txSortedByEffectiveTip{
		txs:     txs,
		baseFee: baseFee,
	}
}

// Len is implementation of sort.Interface
func (t *txSortedByEffectiveTip) Len() int { return len(t.txs) }

// Swap is implementation of sort.Interface
func (t *txSortedByEffectiveTip) Swap(i, j int) {
	t.txs[i], t.txs[j] = t.txs[j], t.txs[i]
}

// Less is implementation of sort.Interface
func (t *txSortedByEffectiveTip) Less(i, j int) bool {
	tip1 := t.txs[i].EffectiveGasTip(t.baseFee)
	tip2 := t.txs[j].EffectiveGasTip(t.baseFee)

	return tip1.Cmp(tip2) < 0
}
