package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

type gasPool struct {
	gas uint64
}

func (g *gasPool) SubGas(amount uint64) error {
	if g.gas < amount {
		return fmt.Errorf("gas limit reached")
	}
	g.gas -= amount
	return nil
}

func newGasPool(gas uint64) *gasPool {
	return &gasPool{gas}
}

var stateTests = "GeneralStateTests"

type stateCase struct {
	Info        *info                `json:"_info"`
	Env         *env                 `json:"env"`
	Pre         stateSnapshop        `json:"pre"`
	Post        map[string]postState `json:"post"`
	Transaction *stTransaction       `json:"transaction"`
}

func RunSpecificTest(t *testing.T, c stateCase, id, fork string, index int, p postEntry) {
	config, ok := Forks[fork]
	if !ok {
		t.Fatalf("config %s not found", fork)
	}

	env := c.Env.ToEnv(t)

	msg, err := c.Transaction.At(p.Indexes)
	if err != nil {
		t.Fatal(err)
	}
	env.GasPrice = msg.GasPrice()

	state, _ := buildState(t, c.Pre)

	forks := config.At(env.Number.Uint64())
	gasTable := config.GasTable(env.Number)

	var root []byte

	txn := state.Txn()

	gasPool := newGasPool(env.GasLimit.Uint64())

	err = txn.Apply(msg, env, gasTable, forks, vmTestBlockHash, gasPool, false)

	// mining rewards
	txn.AddSealingReward(env.Coinbase, big.NewInt(0))

	_, root = txn.Commit(forks.EIP158)

	if !bytes.Equal(root, p.Root.Bytes()) {
		t.Fatalf("root mismatch (%s %d): expected %s but found %s", fork, index, p.Root.String(), hexutil.Encode(root))
	}

	if logs := rlpHash(txn.Logs()); logs != p.Logs {
		t.Fatalf("logs mismatch (%s %d): expected %s but found %s", fork, index, p.Logs.String(), logs.String())
	}
}

func TestState(t *testing.T) {
	long := []string{
		"static_Call50000",
		"static_Return50000",
		"static_Call1MB",
	}

	skip := []string{
		"failed_tx_xcf416c53",
		"RevertOpcodeInCallsOnNonEmptyReturnData",
		"sstore_combinations_initial",
	}

	folders, err := listFolders(stateTests)
	if err != nil {
		t.Fatal(err)
	}

	for _, folder := range folders {
		t.Run(folder, func(t *testing.T) {
			files, err := listFiles(folder)
			if err != nil {
				t.Fatal(err)
			}

			for _, file := range files {
				if !strings.HasSuffix(file, ".json") {
					continue
				}

				if contains(long, file) && testing.Short() {
					t.Skipf("Long tests are skipped in short mode")
					continue
				}

				if contains(skip, file) {
					t.Skip()
					continue
				}

				data, err := ioutil.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}

				var c map[string]stateCase
				if err := json.Unmarshal(data, &c); err != nil {
					t.Fatal(err)
				}

				for _, i := range c {
					for fork, f := range i.Post {
						for indx, e := range f {
							RunSpecificTest(t, i, "id", fork, indx, e)
						}
					}
				}
			}
		})
	}
}
