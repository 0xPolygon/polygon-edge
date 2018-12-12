package tests

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/umbracle/minimal/consensus/ethash"

	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/storage"
)

const blockchainTests = "BlockchainTests"

var none = []string{}

type block struct {
	Header *header   `json:"blockHeader"`
	Number string    `json:"blocknumber"`
	Rlp    string    `json:"rlp"`
	Uncles []*header `json:"uncleHeaders"`
}

type BlockchainTest struct {
	Info *info `json:"_info"`

	Network    string `json:"network"`
	SealEngine string `json:"sealEngine"`

	Blocks        []*block `json:"blocks"`
	LastBlockHash string   `json:"lastblockhash"`

	Genesis    *header `json:"genesisBlockHeader"`
	GenesisRlp string  `json:"genesisRLP"`

	Post stateSnapshop `json:"postState"`
	Pre  stateSnapshop `json:"pre"`
}

// NOTE, only checks the last header, it needs to check the state root too.

func testBlockChainCase(t *testing.T, c *BlockchainTest) {
	_, ok := Forks[c.Network]
	if !ok {
		t.Fatalf("config %s not found", c.Network)
	}

	s, err := storage.NewMemoryStorage(nil)
	if err != nil {
		t.Fatal(err)
	}

	var engine consensus.Consensus
	if c.SealEngine == "NoProof" {
		engine = &consensus.NoProof{}
	} else {
		engine = ethash.NewEthHash(&consensus.ChainConfig{})
	}

	b := blockchain.NewBlockchain(s, engine)
	if err := b.WriteGenesis(c.Genesis.header); err != nil {
		t.Fatal(err)
	}

	// Write blocks
	for _, block := range c.Blocks {
		if block.Header == nil {
			continue
		}

		if err := b.WriteHeader(block.Header.header); err != nil {
			t.Fatal(err)
		}
	}

	if hash := b.Header().Hash().String(); hash != c.LastBlockHash {
		t.Fatalf("header mismatch: found %s but expected %s", hash, c.LastBlockHash)
	}
}

func testBlockChainCases(t *testing.T, folder string, skip []string) {
	files, err := listBlockchainTests(folder)
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		data, err := ioutil.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}

		var bccases map[string]*BlockchainTest
		if err := json.Unmarshal(data, &bccases); err != nil {
			t.Fatal(err)
		}

		for name, cc := range bccases {
			t.Run(name, func(tt *testing.T) {
				if contains(skip, name) {
					tt.Skip()
				} else {
					testBlockChainCase(tt, cc)
				}
			})
		}
	}
}

func TestBlockchainBlockGasLimitTest(t *testing.T) {
	testBlockChainCases(t, "bcBlockGasLimitTest", none)
}

func TestBlockchainExploitTest(t *testing.T) {
	testBlockChainCases(t, "bcExploitTest", none)
}

func TestBlockchainForgedTest(t *testing.T) {
	testBlockChainCases(t, "bcForgedTest", none)
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
	testBlockChainCases(t, "bcMultiChainTest", none)
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
	testBlockChainCases(t, "TransitionTests", none)
}

func listBlockchainTests(folder string) ([]string, error) {
	return listFiles(filepath.Join(blockchainTests, folder))
}
