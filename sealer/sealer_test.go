package sealer

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/umbracle/minimal/state/trie"

	"github.com/umbracle/minimal/consensus/ethash"

	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/blockchain/storage"
	"github.com/umbracle/minimal/chain"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/state"
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

func (h *hookSealer) Author(header *types.Header) (common.Address, error) {
	return common.Address{}, nil
}

func (h *hookSealer) Seal(ctx context.Context, block *types.Block) (*types.Block, error) {
	return h.hook(ctx, block)
}

func (h *hookSealer) Prepare(parent *types.Header, header *types.Header) error {
	return nil
}

func (h *hookSealer) Finalize(txn *state.Txn, block *types.Block) error {
	return nil
}

func (h *hookSealer) Close() error {
	return nil
}

func TestNewBlockWhileSealing(t *testing.T) {
	// If a new block arrives while sealing, the sealing has to stop.

	s, err := storage.NewMemoryStorage(nil)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})

	engine := newHookSealer(func(ctx context.Context, b *types.Block) (*types.Block, error) {
		go func() {
			x := <-ctx.Done()
			fmt.Println("SSSSSSSSSSSSSSSSSSSSSSSSSSSSSS")
			done <- x
		}()
		time.Sleep(10 * time.Second)
		return b, nil
	})

	config := &chain.Params{
		Forks: &chain.Forks{
			EIP155:    chain.NewFork(0),
			Homestead: chain.NewFork(0),
		},
	}

	b := blockchain.NewBlockchain(s, trie.NewMemoryStorage(), engine, config)

	genesis := &chain.Genesis{
		Nonce:      66,
		GasLimit:   5000,
		Difficulty: big.NewInt(17179869184),
		Alloc: chain.GenesisAlloc{
			addr1: chain.GenesisAccount{
				Balance: big.NewInt(10),
			},
		},
	}

	if err := b.WriteGenesis(genesis); err != nil {
		panic(err)
	}

	sealer := NewSealer(DefaultConfig(), nil, b, engine)
	sealer.coinbase = addr2

	fmt.Println(sealer)

	go sealer.commit()
	go sealer.commit()

	<-done
}

func TestSealer(t *testing.T) {
	s, err := storage.NewMemoryStorage(nil)
	if err != nil {
		t.Fatal(err)
	}

	config := &chain.Params{
		Forks: &chain.Forks{
			EIP155:    chain.NewFork(0),
			Homestead: chain.NewFork(0),
		},
	}

	// engine := &boolSealer{}

	engine := ethash.NewEthHash(config, false)
	engine.DatasetDir = "/tmp/example"
	engine.CacheDir = "/tmp/example-cache"

	b := blockchain.NewBlockchain(s, trie.NewMemoryStorage(), engine, config)

	genesis := &chain.Genesis{
		Nonce:      66,
		GasLimit:   5000,
		Difficulty: big.NewInt(17179869184),
		Alloc: chain.GenesisAlloc{
			addr1: chain.GenesisAccount{
				Balance: big.NewInt(10),
			},
		},
	}

	if err := b.WriteGenesis(genesis); err != nil {
		panic(err)
	}

	sealer := NewSealer(DefaultConfig(), nil, b, engine)
	sealer.coinbase = addr2

	fmt.Println(sealer)

	sealer.commit()
}
