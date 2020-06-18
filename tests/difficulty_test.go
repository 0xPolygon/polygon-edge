package tests

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/consensus"

	"github.com/0xPolygon/minimal/types"

	"github.com/0xPolygon/minimal/consensus/ethash"
)

const difficultyTests = "BasicTests"

type difficultyCase struct {
	ParentTimestamp    int64
	ParentDifficulty   uint64
	UncleHash          types.Hash
	CurrentTimestamp   int64
	CurrentBlockNumber uint64
	CurrentDifficulty  uint64
}

func (d *difficultyCase) UnmarshalJSON(input []byte) error {
	type difUnmarshall struct {
		ParentTimestamp    *string     `json:"parentTimestamp"`
		ParentDifficulty   *string     `json:"parentDifficulty"`
		UncleHash          *types.Hash `json:"parentUncles"`
		CurrentTimestamp   *string     `json:"currentTimestamp"`
		CurrentBlockNumber *string     `json:"currentBlockNumber"`
		CurrentDifficulty  *string     `json:"currentDifficulty"`
	}

	var dec difUnmarshall
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}

	var err error

	d.ParentTimestamp, err = types.ParseInt64orHex(dec.ParentTimestamp)
	if err != nil {
		return err
	}
	d.ParentDifficulty, err = types.ParseUint64orHex(dec.ParentDifficulty)
	if err != nil {
		return err
	}
	if dec.UncleHash != nil {
		d.UncleHash = *dec.UncleHash
	}

	d.CurrentTimestamp, err = types.ParseInt64orHex(dec.CurrentTimestamp)
	if err != nil {
		return err
	}
	d.CurrentBlockNumber, err = types.ParseUint64orHex(dec.CurrentBlockNumber)
	if err != nil {
		return err
	}
	d.CurrentDifficulty, err = types.ParseUint64orHex(dec.CurrentDifficulty)
	if err != nil {
		return err
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

var minimumDifficulty = uint64(131072)

func testDifficultyCase(t *testing.T, file string, config *chain.Forks) {
	data, err := ioutil.ReadFile(filepath.Join(TESTS, difficultyTests, file))
	if err != nil {
		t.Fatal(err)
	}

	var cases map[string]*difficultyCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}

	engine, _ := ethash.Factory(context.Background(), &consensus.Config{Params: &chain.Params{Forks: config}})
	engineEthash := engine.(*ethash.Ethash)

	for name, i := range cases {
		if i.ParentDifficulty < minimumDifficulty {
			continue
		}

		parent := &types.Header{
			Difficulty: i.ParentDifficulty,
			Timestamp:  uint64(i.ParentTimestamp),
			Number:     i.CurrentBlockNumber - 1,
			Sha3Uncles: i.UncleHash,
		}

		difficulty := engineEthash.CalcDifficulty(i.CurrentTimestamp, parent)
		if difficulty != i.CurrentDifficulty {
			t.Fatal(name)
		}
	}
}
