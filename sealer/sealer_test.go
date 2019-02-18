package sealer

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/umbracle/minimal/consensus/ethash"

	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/blockchain/storage"
	"github.com/umbracle/minimal/chain"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/state"
)

type boolSealer struct {
}

func (b *boolSealer) VerifyHeader(parent *types.Header, header *types.Header, uncle, seal bool) error {
	return nil
}

func (b *boolSealer) Author(header *types.Header) (common.Address, error) {
	return common.Address{}, nil
}

func (b *boolSealer) Seal(ctx context.Context, block *types.Block) (*types.Block, error) {
	return block, nil
}

func (b *boolSealer) Prepare(parent *types.Header, header *types.Header) error {
	return nil
}

func (b *boolSealer) Finalize(txn *state.Txn, block *types.Block) error {
	return nil
}

func (b *boolSealer) Close() error {
	return nil
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

	b := blockchain.NewBlockchain(s, engine, config)

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

	sealer.commit(false)
}
