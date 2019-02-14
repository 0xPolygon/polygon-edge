package tests

import (
	"encoding/json"
	"io/ioutil"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/umbracle/minimal/chain"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"

	"github.com/umbracle/minimal/consensus/ethash"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
)

const difficultyTests = "BasicTests"

type difficultyCase struct {
	ParentTimestamp    *big.Int
	ParentDifficulty   *big.Int
	UncleHash          common.Hash
	CurrentTimestamp   *big.Int
	CurrentBlockNumber uint64
	CurrentDifficulty  *big.Int
}

func (d *difficultyCase) UnmarshalJSON(input []byte) error {
	type difUnmarshall struct {
		ParentTimestamp    *math.HexOrDecimal256 `json:"parentTimestamp"`
		ParentDifficulty   *math.HexOrDecimal256 `json:"parentDifficulty"`
		UncleHash          *common.Hash          `json:"parentUncles"`
		CurrentTimestamp   *math.HexOrDecimal256 `json:"currentTimestamp"`
		CurrentBlockNumber *math.HexOrDecimal64  `json:"currentBlockNumber"`
		CurrentDifficulty  *math.HexOrDecimal256 `json:"currentDifficulty"`
	}

	var dec difUnmarshall
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}

	if dec.ParentTimestamp != nil {
		d.ParentTimestamp = (*big.Int)(dec.ParentTimestamp)
	}
	if dec.ParentDifficulty != nil {
		d.ParentDifficulty = (*big.Int)(dec.ParentDifficulty)
	}
	if dec.UncleHash != nil {
		d.UncleHash = *dec.UncleHash
	}
	if dec.CurrentTimestamp != nil {
		d.CurrentTimestamp = (*big.Int)(dec.CurrentTimestamp)
	}
	if dec.CurrentBlockNumber != nil {
		d.CurrentBlockNumber = uint64(*dec.CurrentBlockNumber)
	}
	if dec.CurrentDifficulty != nil {
		d.CurrentDifficulty = (*big.Int)(dec.CurrentDifficulty)
	}
	return nil
}

var testnetConfig = &chain.Forks{
	Homestead:      chain.NewFork(0),
	Byzantium:      chain.NewFork(1700000),
	Constantinople: chain.NewFork(4230000),
}

var mainnetConfig = &chain.Forks{
	Homestead: chain.NewFork(1150000),
	Byzantium: chain.NewFork(4370000),
}

func TestDifficultyRopsten(t *testing.T) {
	testDifficultyCase(t, "difficultyRopsten.json", testnetConfig)
}

func TestDifficultyMainNetwork(t *testing.T) {
	testDifficultyCase(t, "difficultyMainNetwork.json", mainnetConfig)
}

func TestDifficultyCustomMainNetwork(t *testing.T) {
	testDifficultyCase(t, "difficultyCustomMainNetwork.json", mainnetConfig)
}

func TestDifficultyMainnet1(t *testing.T) {
	testDifficultyCase(t, "difficulty.json", mainnetConfig)
}

func TestDifficultyHomestead(t *testing.T) {
	testDifficultyCase(t, "difficultyHomestead.json", &chain.Forks{
		Homestead: chain.NewFork(0),
	})
}

func TestDifficultyByzantium(t *testing.T) {
	testDifficultyCase(t, "difficultyByzantium.json", &chain.Forks{
		Byzantium: chain.NewFork(0),
	})
}

func TestDifficultyConstantinople(t *testing.T) {
	testDifficultyCase(t, "difficultyConstantinople.json", &chain.Forks{
		Constantinople: chain.NewFork(0),
	})
}

func testDifficultyCase(t *testing.T, file string, config *chain.Forks) {
	data, err := ioutil.ReadFile(filepath.Join(TESTS, difficultyTests, file))
	if err != nil {
		t.Fatal(err)
	}

	var cases map[string]*difficultyCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}

	engine := ethash.NewEthHash(&chain.Params{Forks: config}, false)
	for name, i := range cases {
		t.Run(name, func(t *testing.T) {
			if i.ParentDifficulty.Cmp(params.MinimumDifficulty) < 0 {
				t.Skip("difficulty below minimum")
				return
			}

			parentNumber := big.NewInt(int64(i.CurrentBlockNumber - 1))

			parent := &types.Header{
				Difficulty: i.ParentDifficulty,
				Time:       i.ParentTimestamp,
				Number:     parentNumber,
				UncleHash:  i.UncleHash,
			}

			difficulty := engine.CalcDifficulty(i.CurrentTimestamp.Uint64(), parent)
			if difficulty.Cmp(i.CurrentDifficulty) != 0 {
				t.Fatal()
			}
		})
	}
}
