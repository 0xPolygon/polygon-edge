package tests

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"strings"
	"testing"

	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/helper/hex"
	"github.com/umbracle/minimal/state"
	"github.com/umbracle/minimal/state/runtime/evm"
	"github.com/umbracle/minimal/state/runtime/precompiled"
	"github.com/umbracle/minimal/types"
)

var stateTests = "GeneralStateTests"

type stateCase struct {
	Info        *info                `json:"_info"`
	Env         *env                 `json:"env"`
	Pre         chain.GenesisAlloc   `json:"pre"`
	Post        map[string]postState `json:"post"`
	Transaction *stTransaction       `json:"transaction"`
}

func RunSpecificTest(file string, t *testing.T, c stateCase, name, fork string, index int, p postEntry) {
	config, ok := Forks[fork]
	if !ok {
		t.Fatalf("config %s not found", fork)
	}

	env := c.Env.ToEnv(t)

	msg, err := c.Transaction.At(p.Indexes)
	if err != nil {
		t.Fatal(err)
	}

	s, _, pastRoot := buildState(t, c.Pre)
	forks := config.At(uint64(env.Number))

	xxx := state.NewExecutor(&chain.Params{Forks: config}, s)
	xxx.SetRuntime(precompiled.NewPrecompiled())
	xxx.SetRuntime(evm.NewEVM())

	xxx.GetHash = func(*types.Header) func(i uint64) types.Hash {
		return vmTestBlockHash
	}

	executor, _ := xxx.BeginTxn(pastRoot, c.Env.ToHeader(t))
	_, _, err = executor.Apply(msg)

	txn := executor.Txn()

	// mining rewards
	txn.AddSealingReward(env.Coinbase, big.NewInt(0))

	_, root := txn.Commit(forks.EIP158)
	if !bytes.Equal(root, p.Root.Bytes()) {
		t.Fatalf("root mismatch (%s %s %d): expected %s but found %s", name, fork, index, p.Root.String(), hex.EncodeToHex(root))
	}

	if logs := rlpHashLogs(txn.Logs()); logs != p.Logs {
		t.Fatalf("logs mismatch (%s, %s %d): expected %s but found %s", name, fork, index, p.Logs.String(), logs.String())
	}
}

func TestState(t *testing.T) {
	long := []string{
		"static_Call50000",
		"static_Return50000",
		"static_Call1MB",
		"stQuadraticComplexityTest",
	}

	skip := []string{
		"failed_tx_xcf416c53",
	}

	// failed_tx_xcf416c53 calls several precompiled contracts (adds the address to the transaction, i.e 'touch')
	// and then reverts. However, in the case of ripemd (0x0...03), it has to keep the precompiled in the transaction.
	// https://github.com/ethereum/yellowpaper/pull/288/files#diff-9f702e1491c55da9d76a68d651278764R2259.
	// This means we have to include some extra functions on the immutable-radix transaction.

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

				for name, i := range c {
					for fork, f := range i.Post {
						for indx, e := range f {
							RunSpecificTest(file, t, i, name, fork, indx, e)
						}
					}
				}
			}
		})
	}
}
