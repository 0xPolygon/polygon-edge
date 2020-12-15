package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/0xPolygon/minimal/blockchain/storage/memory"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/consensus/ethash"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/types"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/state"
	itrie "github.com/0xPolygon/minimal/state/immutable-trie"
	"github.com/0xPolygon/minimal/state/runtime/evm"
	"github.com/0xPolygon/minimal/state/runtime/precompiled"
)

const blockchainTests = "BlockchainTests"

var none = []string{}

type block struct {
	Header *header   `json:"blockHeader"`
	Number string    `json:"blocknumber"`
	Rlp    string    `json:"rlp"`
	Uncles []*header `json:"uncleHeaders"`
}

func (b *block) decode() (*types.Block, error) {
	data, err := hex.DecodeHex(b.Rlp)
	if err != nil {
		return nil, err
	}
	var bb1 types.Block
	if err := bb1.UnmarshalRLP(data); err != nil {
		return nil, err
	}
	return &bb1, err
}

type BlockchainTest struct {
	Info *info `json:"_info"`

	Network    string `json:"network"`
	SealEngine string `json:"sealEngine"`

	Blocks        []*block `json:"blocks"`
	LastBlockHash string   `json:"lastblockhash"`

	Genesis    *header `json:"genesisBlockHeader"`
	GenesisRlp string  `json:"genesisRLP"`

	Post chain.GenesisAlloc `json:"postState"`
	Pre  chain.GenesisAlloc `json:"pre"`
}

func (b *BlockchainTest) buildGenesis() *chain.Genesis {
	return &chain.Genesis{
		Nonce:      b.Genesis.header.Nonce,
		Timestamp:  b.Genesis.header.Timestamp,
		ParentHash: b.Genesis.header.ParentHash,
		ExtraData:  b.Genesis.header.ExtraData,
		GasLimit:   b.Genesis.header.GasLimit,
		GasUsed:    b.Genesis.header.GasUsed,
		Difficulty: b.Genesis.header.Difficulty,
		Mixhash:    b.Genesis.header.MixHash,
		Coinbase:   b.Genesis.header.Miner,
		Alloc:      b.Pre,
	}
}

func testBlockChainCase(t *testing.T, c *BlockchainTest) {
	config, ok := Forks[c.Network]
	if !ok {
		t.Fatalf("config %s not found", c.Network)
	}

	// builtins := buildBuiltins(t, config)
	s, err := memory.NewMemoryStorage(nil)
	if err != nil {
		t.Fatal(err)
	}

	params := &chain.Params{Forks: config, ChainID: 1}

	var fakePow bool
	if c.SealEngine == "NoProof" {
		fakePow = true
	} else {
		fakePow = false
	}

	engine, _ := ethash.Factory(context.Background(), &consensus.Config{Params: params}, nil, nil, nil)
	if fakePow {
		engine.(*ethash.Ethash).SetFakePow()
	}

	genesis := c.buildGenesis()

	st := itrie.NewState(itrie.NewMemoryStorage())

	executor := state.NewExecutor(params, st)
	executor.SetRuntime(precompiled.NewPrecompiled())
	executor.SetRuntime(evm.NewEVM())

	b := blockchain.NewBlockchain(s, params, engine, executor)
	if err := b.WriteGenesis(genesis); err != nil {
		t.Fatal(err)
	}

	executor.GetHash = b.GetHashHelper

	// Change the dao block
	if c.Network == "HomesteadToDaoAt5" {
		b.Executor().SetDAOHardFork(5)
		// b.SetDAOBlock(5)
		engine.(*ethash.Ethash).SetDAOBlock(5)
	}

	// Validate the genesis
	genesisHeader, ok := b.GetHeaderByNumber(0)
	if !ok {
		t.Fatal("not found")
	}
	// b.SetPrecompiled(builtins)
	if hash := genesisHeader.Hash; hash != c.Genesis.header.Hash {
		t.Fatalf("genesis hash mismatch: expected %s but found %s", c.Genesis.header.Hash, hash.String())
	}
	if stateRoot := genesisHeader.StateRoot; stateRoot != c.Genesis.header.StateRoot {
		t.Fatalf("genesis state root mismatch: expected %s but found %s", c.Genesis.header.StateRoot.String(), stateRoot.String())
	}

	// Write blocks
	validBlocks := map[types.Hash]*types.Block{}
	for _, entry := range c.Blocks {
		block, err := entry.decode()
		if err != nil {
			if entry.Header == nil {
				continue
			} else {
				t.Fatalf("Failed to decode rlp block: %v", err)
			}
		}

		blocks := []*types.Block{block}
		if err := b.WriteBlocks(blocks); err != nil {
			if entry.Header == nil {
				continue
			} else {
				t.Fatalf("Failed to insert block: %v", err)
			}
		}
		if entry.Header == nil {
			t.Fatal("Block insertion should have failed")
		}
		validBlocks[block.Hash()] = block
	}

	lastBlock, _ := b.Header()
	// Validate last block
	if hash := lastBlock.Hash.String(); hash != c.LastBlockHash {
		t.Fatalf("header mismatch: found %s but expected %s", hash, c.LastBlockHash)
	}

	snap, err := st.NewSnapshotAt(lastBlock.StateRoot)
	if err != nil {
		t.Fatalf("state of last block not found")
	}

	// Validate post state, TODO account state and code
	// txn := state.Txn()
	txn := state.NewTxn(st, snap)

	for k, v := range c.Post {
		obj, ok := txn.GetAccount(k)
		if !ok {
			t.Fatalf("account %s not found", k.String())
		}
		if code := txn.GetCode(k); !bytes.Equal(v.Code, code) {
			t.Fatal()
		}
		if v.Balance.Cmp(obj.Balance) != 0 {
			t.Fatal()
		}
		if v.Nonce != obj.Nonce {
			t.Fatal()
		}
	}

	// Validate imported headers
	header, _ := b.Header()
	for current := header; current != nil && current.Number != 0; current, _ = b.GetHeaderByHash(current.ParentHash) {
		valid, ok := validBlocks[current.Hash]
		if !ok {
			t.Fatalf("Block from chain %s not found", current.Hash)
		}
		if !reflect.DeepEqual(current, valid.Header) {
			t.Fatalf("Headers are not equal")
		}
	}
}

func testBlockChainCases(t *testing.T, folder string, skip []string) {
	files, err := listBlockchainTests(folder)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			data, err := ioutil.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}

			var bccases map[string]*BlockchainTest
			if err := json.Unmarshal(data, &bccases); err != nil {
				t.Fatal(err)
			}

			for name, cc := range bccases {
				if !contains(skip, name) {
					testBlockChainCase(t, cc)
				}
			}
		})
	}
}

func TestBlockchainInvalidBlocks(t *testing.T) {
	testBlockChainCases(t, "InvalidBlocks", []string{})
}

func TestBlockchainValidBlocks(t *testing.T) {
	testBlockChainCases(t, "ValidBlocks", []string{})
}

func TestBlockchainTransitionTests(t *testing.T) { // x
	testBlockChainCases(t, "TransitionTests", []string{
		"blockChainFrontier", // TODO
	})
}

func listBlockchainTests(folder string) ([]string, error) {
	return listFiles(filepath.Join(blockchainTests, folder))
}
