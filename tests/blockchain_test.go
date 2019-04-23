package tests

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/blockchain/storage/memory"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/consensus/ethash"
	"github.com/umbracle/minimal/state"
	trie "github.com/umbracle/minimal/state/immutable-trie"

	"github.com/umbracle/minimal/blockchain"
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
	data, err := hexutil.Decode(b.Rlp)
	if err != nil {
		return nil, err
	}
	var bb types.Block
	err = rlp.DecodeBytes(data, &bb)
	return &bb, err
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
		Nonce:      b.Genesis.header.Nonce.Uint64(),
		Timestamp:  b.Genesis.header.Time.Uint64(),
		ParentHash: b.Genesis.header.ParentHash,
		ExtraData:  b.Genesis.header.Extra,
		GasLimit:   b.Genesis.header.GasLimit,
		GasUsed:    b.Genesis.header.GasUsed,
		Difficulty: b.Genesis.header.Difficulty,
		Mixhash:    b.Genesis.header.MixDigest,
		Coinbase:   b.Genesis.header.Coinbase,
		Alloc:      b.Pre,
	}
}

func testBlockChainCase(t *testing.T, c *BlockchainTest) {
	config, ok := Forks[c.Network]
	if !ok {
		t.Fatalf("config %s not found", c.Network)
	}

	builtins := buildBuiltins(t, config)
	s, err := memory.NewMemoryStorage(nil)
	if err != nil {
		t.Fatal(err)
	}

	params := &chain.Params{Forks: config}

	var fakePow bool
	if c.SealEngine == "NoProof" {
		fakePow = true
	} else {
		fakePow = false
	}

	engine := ethash.NewEthHash(params, fakePow)
	genesis := c.buildGenesis()

	st := trie.NewState(trie.NewMemoryStorage())

	b := blockchain.NewBlockchain(s, st, engine, params)
	if err := b.WriteGenesis(genesis); err != nil {
		t.Fatal(err)
	}

	b.SetPrecompiled(builtins)
	if hash := b.Genesis().Hash(); hash != c.Genesis.header.Hash() {
		t.Fatalf("genesis hash mismatch: expected %s but found %s", c.Genesis.header.Hash(), hash.String())
	}
	if stateRoot := b.Genesis().Root; stateRoot != c.Genesis.header.Root {
		t.Fatalf("genesis state root mismatch: expected %s but found %s", c.Genesis.header.Root.String(), stateRoot.String())
	}

	// Write blocks
	validBlocks := map[common.Hash]*types.Block{}
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

		// validate header
		header, _ := b.Header()
		if !reflect.DeepEqual(entry.Header.header, header) {
			t.Fatal("Header is not correct")
		}
		validBlocks[block.Hash()] = block
	}

	lastBlock, _ := b.Header()
	// Validate last block
	if hash := lastBlock.Hash().String(); hash != c.LastBlockHash {
		t.Fatalf("header mismatch: found %s but expected %s", hash, c.LastBlockHash)
	}

	snap, ok := b.GetState(lastBlock)
	if !ok {
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
	for current := header; current != nil && current.Number.Uint64() != 0; current, _ = b.GetHeaderByHash(current.ParentHash) {
		valid, ok := validBlocks[current.Hash()]
		if !ok {
			t.Fatalf("Block from chain %s not found", current.Hash())
		}
		if !reflect.DeepEqual(current, valid.Header()) {
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

func TestBlockchainBlockGasLimitTest(t *testing.T) {
	testBlockChainCases(t, "bcBlockGasLimitTest", none)
}

func TestBlockchainExploitTest(t *testing.T) {
	if !testing.Short() {
		testBlockChainCases(t, "bcExploitTest", none)
	}
}

func TestBlockchainForgedTest(t *testing.T) {
	testBlockChainCases(t, "bcForgedTest", []string{
		"ForkUncle",
	})
}

func TestBlockchainForkStressTest(t *testing.T) {
	testBlockChainCases(t, "bcForkStressTest", none)
}

func TestBlockchainGasPricerTest(t *testing.T) {
	testBlockChainCases(t, "bcGasPricerTest", none)
}

func TestBlockchainInvalidHeaderTest(t *testing.T) {
	testBlockChainCases(t, "bcInvalidHeaderTest", none)
}

func TestBlockchainMultiChainTest(t *testing.T) {
	testBlockChainCases(t, "bcMultiChainTest", []string{
		"ChainAtoChainB_blockorder",
		"CallContractFromNotBestBlock",
	})
}

func TestBlockchainRandomBlockhashTest(t *testing.T) {
	testBlockChainCases(t, "bcRandomBlockhashTest", none)
}

func TestBlockchainStateTests(t *testing.T) {
	testBlockChainCases(t, "bcStateTests", none)
}

func TestBlockchainTotalDifficulty(t *testing.T) {
	testBlockChainCases(t, "bcTotalDifficultyTest", []string{
		"lotsOfLeafs",
		"lotsOfBranches",
		"sideChainWithMoreTransactions",
		"uncleBlockAtBlock3afterBlock4", // TODO
	})
}

func TestBlockchainUncleHeaderValidity(t *testing.T) {
	testBlockChainCases(t, "bcUncleHeaderValidity", none)
}

func TestBlockchainUncleTest(t *testing.T) {
	testBlockChainCases(t, "bcUncleTest", none)
}

func TestBlockchainValidBlockTest(t *testing.T) {
	testBlockChainCases(t, "bcValidBlockTest", none)
}

func TestBlockchainWallet(t *testing.T) {
	testBlockChainCases(t, "bcWalletTest", none)
}

func TestBlockchainTransitionTests(t *testing.T) {
	testBlockChainCases(t, "TransitionTests", []string{
		"blockChainFrontier",
		"DaoTransactions", // TODO
	})
}

func listBlockchainTests(folder string) ([]string, error) {
	return listFiles(filepath.Join(blockchainTests, folder))
}
