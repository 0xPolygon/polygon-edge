package ethash

import (
	"context"
	crand "crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"runtime"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"golang.org/x/crypto/sha3"
)

const (
	// staleThreshold is the maximum depth of the acceptable stale but valid ethash solution.
	staleThreshold = 7
)

var (
	errNoMiningWork      = errors.New("no mining work available yet")
	errInvalidSealResult = errors.New("invalid or stale proof-of-work solution")
)

func (e *Ethash) Prepare(parent *types.Header, header *types.Header) error {
	header.Difficulty = e.CalcDifficulty(header.Time.Uint64(), parent)
	return nil
}

func (e *Ethash) Seal(ctx context.Context, block *types.Block) (*types.Block, error) {
	fmt.Println("- seal -")

	// Create a runner and the multiple search threads it directs
	abort := make(chan struct{})

	e.lock.Lock()
	threads := e.threads
	if e.rand == nil {
		seed, err := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
		if err != nil {
			e.lock.Unlock()
			return nil, err
		}
		e.rand = rand.New(rand.NewSource(seed.Int64()))
	}
	e.lock.Unlock()
	if threads == 0 {
		threads = runtime.NumCPU()
	}
	if threads < 0 {
		threads = 0 // Allows disabling local mining without extra logic around local/remote
	}

	var (
		pend   sync.WaitGroup
		locals = make(chan *types.Block)
	)

	threads = 1

	for i := 0; i < threads; i++ {
		pend.Add(1)
		go func(id int, nonce uint64) {
			defer pend.Done()
			e.mine(block, id, nonce, abort, locals)
		}(i, uint64(e.rand.Int63()))
	}

	resultCh := make(chan *types.Block, 1)

	// Wait until sealing is terminated or a nonce is found
	go func() {
		var result *types.Block
		select {
		case <-ctx.Done():
			// Outside abort, stop all miner threads
			close(abort)
		case result = <-locals:
			// One of the threads found a block, abort all others
			close(abort)
		}
		// Wait for all miners to terminate and return the block
		pend.Wait()
		resultCh <- result
	}()

	res := <-resultCh
	return res, nil
}

// SealHash returns the hash of a block prior to it being sealed.
func (e *Ethash) sealHash(header *types.Header) (hash common.Hash) {
	hasher := sha3.NewLegacyKeccak256()

	rlp.Encode(hasher, []interface{}{
		header.ParentHash,
		header.UncleHash,
		header.Coinbase,
		header.Root,
		header.TxHash,
		header.ReceiptHash,
		header.Bloom,
		header.Difficulty,
		header.Number,
		header.GasLimit,
		header.GasUsed,
		header.Time,
		header.Extra,
	})
	hasher.Sum(hash[:0])
	return hash
}

// mine is the actual proof-of-work miner that searches for a nonce starting from
// seed that results in correct final block difficulty.
func (e *Ethash) mine(block *types.Block, id int, seed uint64, abort chan struct{}, found chan *types.Block) {
	// Extract some data from the header
	fmt.Println("- mine -")

	header := block.Header()
	hash := e.sealHash(header).Bytes()
	target := new(big.Int).Div(two256, header.Difficulty)
	number := header.Number.Uint64()
	dataset := e.dataset(number, false)

	// Start generating random nonces until we abort or find a good one
	var (
		attempts = int64(0)
		nonce    = seed
	)
	logger := log.New("miner", id)
	logger.Trace("Started ethash search for new nonces", "seed", seed)

search:
	for {
		select {
		case <-abort:
			// Mining terminated, update stats and abort
			logger.Trace("Ethash nonce search aborted", "attempts", nonce-seed)
			break search

		default:
			// We don't have to update hash rate on every nonce, so update after after 2^X nonces
			attempts++
			if (attempts % (1 << 15)) == 0 {
				attempts = 0
			}
			// Compute the PoW value of this nonce
			digest, result := hashimotoFull(dataset.dataset, hash, nonce)
			if new(big.Int).SetBytes(result).Cmp(target) <= 0 {
				// Correct nonce found, create a new header with it
				header = types.CopyHeader(header)
				header.Nonce = types.EncodeNonce(nonce)
				header.MixDigest = common.BytesToHash(digest)

				// Seal and return a block (if still needed)
				select {
				case found <- block.WithSeal(header):
					logger.Trace("Ethash nonce found and reported", "attempts", nonce-seed, "nonce", nonce)
				case <-abort:
					logger.Trace("Ethash nonce found but discarded", "attempts", nonce-seed, "nonce", nonce)
				}
				break search
			}
			nonce++
		}
	}
	// Datasets are unmapped in a finalizer. Ensure that the dataset stays live
	// during sealing so it's not unmapped while being read.
	runtime.KeepAlive(dataset)
}
