package tests

import (
	"encoding/json"
	"io/ioutil"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/core"
	transition "github.com/umbracle/minimal/state"
)

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

	state := buildNewState(t, c.Pre)

	gaspool := new(core.GasPool)
	gaspool.AddGas(env.GasLimit.Uint64())

	forks := config.At(env.Number.Uint64())

	tt := &transition.Transition{
		State:    state,
		Env:      env,
		Config:   forks,
		GasTable: config.GasTable(env.Number),
		Msg:      msg,
		Gp:       gaspool,
		GetHash:  vmTestBlockHash,
	}

	snapshot := state.Snapshot()
	if err := tt.Apply(); err != nil {
		state.RevertToSnapshot(snapshot)
	}

	state.AddBalance(env.Coinbase, new(big.Int))

	/*
		root := state.IntermediateRoot(forks.EIP158)

		if root != p.Root {
			t.Fatalf("root mismatch (%s %d): expected %s but found %s", fork, index, p.Root.String(), root.String())
		}
	*/

	//fmt.Println("-- logs --")
	//fmt.Println(state.Logs())

	if logs := rlpHash(state.Logs()); logs != p.Logs {
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
		"RevertPrecompiledTouch",
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

							/*
								fmt.Println("-----------")
								fmt.Println(file)
								fmt.Println(fork)
								fmt.Println(indx)
								fmt.Println(id)
							*/

							RunSpecificTest(t, i, "id", fork, indx, e)
						}
					}
				}
			}
		})
	}
}
