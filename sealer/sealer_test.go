package sealer

import (
	"context"
	"encoding/binary"
	"io/ioutil"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/blockchain/storage/memory"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/consensus"
	itrie "github.com/0xPolygon/minimal/state/immutable-trie"
	"github.com/0xPolygon/minimal/state/runtime/evm"
	"github.com/0xPolygon/minimal/state/runtime/precompiled"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"

	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/types"
)

type sealHook func(ctx context.Context, block *types.Block) (*types.Block, error)

type hookSealer struct {
	hook sealHook
}

func newHookSealer(hook sealHook) *hookSealer {
	return &hookSealer{hook}
}

func (h *hookSealer) VerifyHeader(chain consensus.ChainReader, header *types.Header, uncle, seal bool) error {
	return nil
}

func (h *hookSealer) Seal(chain consensus.ChainReader, block *types.Block, ctx context.Context) (*types.Block, error) {
	return h.hook(ctx, block)
}

func (h *hookSealer) Close() error {
	return nil
}

func testSealer(t *testing.T, sealerConfig *Config, hook sealHook) (*Sealer, func()) {
	storage, err := memory.NewMemoryStorage(nil)
	assert.NoError(t, err)

	engine := newHookSealer(hook)
	config := &chain.Params{
		Forks: &chain.Forks{
			EIP155:    chain.NewFork(0),
			Homestead: chain.NewFork(0),
			Byzantium: chain.NewFork(0),
		},
	}

	st := itrie.NewState(itrie.NewMemoryStorage())

	executor := state.NewExecutor(config, st)
	executor.SetRuntime(precompiled.NewPrecompiled())
	executor.SetRuntime(evm.NewEVM())

	b := blockchain.NewBlockchain(storage, config, engine, executor)

	executor.GetHash = b.GetHashHelper

	advanceChain := func() {
		header, _ := b.Header()
		parent := header
		num := parent.Number

		newHeader := &types.Header{
			ParentHash: parent.Hash,
			Number:     num + 1,
			GasLimit:   8000000,
			ExtraData:  []byte{},
			Difficulty: 10,
			StateRoot:  types.EmptyRootHash,
		}

		newHeader.ComputeHash()
		if err := b.WriteHeader(newHeader); err != nil {
			t.Fatal(err)
		}
	}

	nonce := uint64(66)

	genesis := &chain.Genesis{
		GasLimit:   5000,
		Difficulty: 17179869184,
		Alloc: chain.GenesisAlloc{
			addr1: chain.GenesisAccount{
				Balance: big.NewInt(10),
			},
		},
	}
	binary.BigEndian.PutUint64(genesis.Nonce[:], nonce)

	if err := b.WriteGenesis(genesis); err != nil {
		panic(err)
	}

	logger := hclog.New(&hclog.LoggerOptions{
		Output: ioutil.Discard,
	})

	sealerConfig.Coinbase = addr2
	sealer := NewSealer(sealerConfig, logger, b, engine, executor)

	return sealer, advanceChain
}

func TestCancelSealingFunction(t *testing.T) {
	seal := make(chan chan struct{})
	s, _ := testSealer(t, DefaultConfig(), func(ctx context.Context, b *types.Block) (*types.Block, error) {
		ok := make(chan struct{})
		seal <- ok
		<-ok
		return b, nil
	})

	doSeal := func(ctx context.Context, cancel context.CancelFunc) {
		ch := s.sealAsync(ctx)

		ok := <-seal
		cancel()
		ok <- struct{}{}

		<-ch

		// it should not include any new block
		if header, _ := s.blockchain.Header(); header.Number != 0 {
			t.Fatal("bad")
		}
	}

	// cancel the seal context
	ctx, cancel := context.WithCancel(context.Background())
	doSeal(ctx, cancel)

	// cancel the parent context
	ctx, cancel = context.WithCancel(context.Background())
	subCtx, _ := context.WithCancel(ctx)
	doSeal(subCtx, cancel)
}

func TestCancelSealingRoutine(t *testing.T) {
	seal := make(chan chan struct{})
	s, _ := testSealer(t, DefaultConfig(), func(ctx context.Context, b *types.Block) (*types.Block, error) {
		ok := make(chan struct{})
		seal <- ok
		<-ok
		return b, nil
	})

	// start the sealer
	s.SetEnabled(true)
	ch := <-seal

	// stop the sealer
	s.SetEnabled(false)
	ch <- struct{}{} // finish block sealing

	// wait for the run routine to finish
	time.Sleep(500 * time.Millisecond)

	// it should not include any new block
	if header, _ := s.blockchain.Header(); header.Number != 0 {
		t.Fatal("bad")
	}
}

func TestCancelSealingOnAdvanceChain(t *testing.T) {
	// tests that the async sealing routines get cancelled if there is a new head block

	seal := make(chan chan struct{})
	s, advance := testSealer(t, DefaultConfig(), func(ctx context.Context, b *types.Block) (*types.Block, error) {
		ok := make(chan struct{})
		seal <- ok
		<-ok
		return b, nil
	})

	s.SetEnabled(true)

	// listen for sealing events
	resCh := make(chan *SealedNotify)
	go func() {
		for {
			resCh <- <-s.SealedCh
		}
	}()

	// sealing block 1
	wait1 := <-seal

	// advance block 1 (the sealer has to discard current block 1 sealing)
	advance()

	// sealing block 2
	wait2 := <-seal

	// release both sealers
	wait1 <- struct{}{}
	wait2 <- struct{}{}

	// expect only one event (block 2 sealed)
	select {
	case evnt := <-resCh:
		if evnt.Block.Number() != 2 {
			t.Fatal("incorrect sealed block")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("no sealing events")
	}

	// do not expect more events
	select {
	case <-resCh:
		t.Fatal("more sealing events not expected")
	default:
	}
}

func TestSealBlockAfterTxnInDevMode(t *testing.T) {
	// tests that the sealer waits for new transactions in dev-mode

	notify := make(chan struct{})
	s, _ := testSealer(t, &Config{DevMode: true}, func(ctx context.Context, b *types.Block) (*types.Block, error) {
		notify <- struct{}{}
		return b, nil
	})

	s.SetEnabled(true)

	// It should not process txns yet
	select {
	case <-notify:
		t.Fatal("sealing not expected")
	case <-time.After(100 * time.Millisecond):
	}

	if err := s.AddTx(&types.Transaction{From: types.StringToAddress("1"), Gas: 100000000}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-notify:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("sealing expected")
	}
	time.Sleep(1 * time.Second)
}

func noopHookfunc(ctx context.Context, b *types.Block) (*types.Block, error) {
	return b, nil
}

func TestAcceptNonSignedTransactionsOnlyInDevMode(t *testing.T) {
	s, _ := testSealer(t, &Config{DevMode: false}, noopHookfunc)

	txn := &types.Transaction{From: types.StringToAddress("1")}
	if err := s.AddTx(txn); err == nil {
		t.Fatal("Sealer in non dev-mode cannot accept non-signed transactions")
	}

	s, _ = testSealer(t, &Config{DevMode: true}, noopHookfunc)
	if err := s.AddTx(txn); err != nil {
		t.Fatal(err)
	}
}
