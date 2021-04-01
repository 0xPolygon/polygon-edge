package pow

import (
	"context"
	"crypto/ecdsa"
	crand "crypto/rand"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/txpool"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
)

var (
	two256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))
)

// Pow is a vanilla proof-of-work engine
type Pow struct {
	logger     hclog.Logger
	blockchain *blockchain.Blockchain
	executor   *state.Executor
	min        uint64
	max        uint64
	closeCh    chan struct{}
}

func Factory(ctx context.Context, config *consensus.Config, txpool *txpool.TxPool, blockchain *blockchain.Blockchain, executor *state.Executor, privateKey *ecdsa.PrivateKey, logger hclog.Logger) (consensus.Consensus, error) {
	p := &Pow{
		logger:     logger.Named("pow"),
		blockchain: blockchain,
		executor:   executor,
		min:        1000000,
		max:        1500000,
		closeCh:    make(chan struct{}),
	}
	return p, nil
}

func (p *Pow) StartSeal() {
	go p.run()
}

func (p *Pow) run() {
	p.logger.Info("started")

	sub := p.blockchain.SubscribeEvents()
	eventCh := sub.GetEventCh()

	for {
		// start sealing
		parent := p.blockchain.Header()

		target := parent.Number + 1
		p.logger.Debug("sealing block", "num", target)

		subCtx, cancel := context.WithCancel(context.Background())
		done := p.sealAsync(subCtx, parent)

		// wait for the sealing to be done
	WAIT:
		select {
		case <-done:
			// the sealing process has finished
		case <-p.closeCh:
			// the sealing routine has been canceled
			cancel()
			return
		case evnt := <-eventCh:
			// only restart if there is a new head
			if evnt.Type == blockchain.EventFork {
				goto WAIT
			}
			if evnt.Header().Equal(parent) {
				// there will be an event when the sealer writes the block
				// we have to avoid it because we will get notified of that
				// with the done channel
				goto WAIT
			}
			p.logger.Debug("restart sealing due to new blocks")
		}

		// cancel the sealing process context
		cancel()
	}

}

func (p *Pow) VerifyHeader(parent *types.Header, header *types.Header) error {
	if header.Timestamp <= parent.Timestamp {
		return fmt.Errorf("timestamp lower or equal than parent")
	}
	if header.Difficulty < p.min {
		return fmt.Errorf("difficulty not correct. '%d' <! '%d'", header.Difficulty, p.min)
	}
	if header.Difficulty > p.max {
		return fmt.Errorf("difficulty not correct. '%d' >! '%d'", header.Difficulty, p.min)
	}
	return nil
}

func (p *Pow) sealAsync(ctx context.Context, parent *types.Header) chan struct{} {
	ch := make(chan struct{})
	go func() {
		if err := p.seal(ctx, parent); err != nil {
			p.logger.Trace("failed to seal", "err", err)
		}
		select {
		case ch <- struct{}{}:
		default:
		}
	}()
	return ch
}

func (p *Pow) seal(ctx context.Context, parent *types.Header) error {
	num := parent.Number
	header := &types.Header{
		ParentHash: parent.Hash,
		Number:     num + 1,
		GasLimit:   100000000, // placeholder for now
		Timestamp:  uint64(time.Now().Unix()),
	}

	transition, err := p.executor.BeginTxn(parent.StateRoot, header)
	if err != nil {
		return err
	}

	_, root := transition.Commit()

	header.StateRoot = root
	header.GasUsed = transition.TotalGas()

	// header hash is computed inside buildBlock
	block := consensus.BuildBlock(header, nil, transition.Receipts())

	// seal the block
	block, err = p.sealImpl(ctx, block)
	if err != nil {
		return err
	}
	if block == nil {
		return nil
	}
	if err := p.blockchain.WriteBlocks([]*types.Block{block}); err != nil {
		return err
	}
	p.logger.Info("block sealed", "num", parent.Number+1)
	return nil
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (p *Pow) Prepare(header *types.Header) error {
	return nil
}

func (p *Pow) sealImpl(ctx context.Context, block *types.Block) (*types.Block, error) {
	header := block.Header
	header.Difficulty = randomInt(p.min, p.max)

	seed, err := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return nil, err
	}

	rand := rand.New(rand.NewSource(seed.Int64()))
	nonce := uint64(rand.Int63())

	target := new(big.Int).Div(two256, new(big.Int).SetUint64(header.Difficulty))
	aux := big.NewInt(1)

	for {
		header.SetNonce(nonce)
		header.ComputeHash()

		aux.SetBytes(header.Hash.Bytes())
		if aux.Cmp(target) < 0 {
			break
		}
		nonce++

		if ctx.Err() != nil {
			return nil, nil
		}
	}

	block = &types.Block{
		Header:       header.Copy(),
		Transactions: block.Transactions,
	}
	return block, nil
}

func (p *Pow) Seal(block *types.Block, ctx context.Context) (*types.Block, error) {
	return nil, nil
}

func (p *Pow) Close() error {
	return nil
}

func randomInt(min, max uint64) uint64 {
	rand.Seed(time.Now().UnixNano())
	return min + uint64(rand.Intn(int(max-min)))
}
