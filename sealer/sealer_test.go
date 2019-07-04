package sealer

import (
	"context"
	"encoding/binary"
	"io/ioutil"
	"math/big"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/blockchain/storage/memory"
	"github.com/umbracle/minimal/chain"
	itrie "github.com/umbracle/minimal/state/immutable-trie"

	"github.com/umbracle/minimal/state"
	"github.com/umbracle/minimal/types"
)

type sealHook func(ctx context.Context, block *types.Block) (*types.Block, error)

type hookSealer struct {
	hook sealHook
}

func newHookSealer(hook sealHook) *hookSealer {
	return &hookSealer{hook}
}

func (h *hookSealer) VerifyHeader(parent *types.Header, header *types.Header, uncle, seal bool) error {
	return nil
}

func (h *hookSealer) Author(header *types.Header) (types.Address, error) {
	return types.Address{}, nil
}

func (h *hookSealer) Seal(ctx context.Context, block *types.Block) (*types.Block, error) {
	return h.hook(ctx, block)
}

func (h *hookSealer) Prepare(parent *types.Header, header *types.Header) error {
	header.Difficulty = 0
	return nil
}

func (h *hookSealer) Finalize(txn *state.Txn, block *types.Block) error {
	return nil
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
		},
	}

	st := itrie.NewState(itrie.NewMemoryStorage())
	b := blockchain.NewBlockchain(storage, st, engine, config)

	advanceChain := func() {
		header, _ := b.Header()
		parent := header
		num := parent.Number

		newHeader := &types.Header{
			ParentHash: parent.Hash(),
			Number:     num + 1,
			GasLimit:   calcGasLimit(parent, 8000000, 8000000),
			ExtraData:  []byte{},
			Difficulty: 10,
		}

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

	sealer := NewSealer(sealerConfig, logger, b, engine)
	sealer.coinbase = addr2

	return sealer, advanceChain
}

func TestSealerContextCancel(t *testing.T) {
	t.Skip()

	// If a new block arrives while sealing, the sealing has to stop.

	done := make(chan struct{})

	sealer, _ := testSealer(t, DefaultConfig(), func(ctx context.Context, b *types.Block) (*types.Block, error) {
		go func() {
			done <- <-ctx.Done()
		}()
		time.Sleep(2 * time.Second)
		return b, nil
	})

	go sealer.commit()
	time.Sleep(1 * time.Second)
	go sealer.commit()

	<-done
	<-sealer.SealedCh

	select {
	case <-sealer.SealedCh:
		// Only one value expected
		t.Fatal("bad")
	default:
	}
}

func TestSealerNotifyNewBlock(t *testing.T) {
	t.Skip()

	// If we get a notification of a new block while sealing we stop the process

	done := make(chan struct{})
	sealer, advance := testSealer(t, DefaultConfig(), func(ctx context.Context, b *types.Block) (*types.Block, error) {
		go func() {
			done <- <-ctx.Done()
		}()
		time.Sleep(2 * time.Second)
		return b, nil
	})

	go sealer.run(context.Background())

	// Notify to start mining (otherwise it will wait until time interval)
	advance()

	time.Sleep(1 * time.Second)

	// Notify again to cancel current sealing
	advance()

	<-done
	<-sealer.SealedCh

	select {
	case <-sealer.SealedCh:
		// Only one value expected
		t.Fatal("bad")
	default:
	}
}

func TestSealerPeriodicSealing(t *testing.T) {
	t.Skip()

	// it has to seal blocks periodically

	done := make(chan struct{})
	interval := 1 * time.Second

	sealerConfig := DefaultConfig()
	sealerConfig.CommitInterval = interval

	sealer, _ := testSealer(t, sealerConfig, func(ctx context.Context, b *types.Block) (*types.Block, error) {
		go func() {
			done <- <-ctx.Done()
		}()
		// time.Sleep(sealerTime)
		return b, nil
	})

	sealer.SetEnabled(true)

	for i := 0; i < 5; i++ {
		last := time.Now()

		select {
		case b := <-sealer.SealedCh:
			if b.Block.Number() != uint64(i+1) {
				t.Fatal("bad")
			}
			if time.Since(last) > 2*time.Second {
				t.Fatal("bad")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("bad")
		}
	}

	sealer.SetEnabled(false)
	if sealer.enabled == true {
		t.Fatal("bad")
	}

	select {
	case <-sealer.SealedCh:
		t.Fatal("bad")
	default:
	}
}
