package tests

import (
	"encoding/json"
	"io/ioutil"
	"math/big"
	"strings"
	"testing"

	"github.com/umbracle/minimal/chain"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/umbracle/minimal/state/evm"
)

var mainnetChainConfig = chain.Params{
	Forks: &chain.Forks{
		Homestead: chain.NewFork(1150000),
		EIP150:    chain.NewFork(2463000),
		EIP158:    chain.NewFork(2675000),
		Byzantium: chain.NewFork(4370000),
	},
}

var vmTests = "VMTests"

type VMCase struct {
	Info *info `json:"_info"`
	Env  *env  `json:"env"`
	Exec *exec `json:"exec"`

	Gas  string `json:"gas"`
	Logs string `json:"logs"`
	Out  string `json:"out"`

	Post chain.GenesisAlloc `json:"post"`
	Pre  chain.GenesisAlloc `json:"pre"`
}

func testVMCase(t *testing.T, name string, c *VMCase) {
	env := c.Env.ToEnv(t)
	env.GasPrice = c.Exec.GasPrice

	initialCall := true
	canTransfer := func(state evm.State, address common.Address, amount *big.Int) bool {
		if initialCall {
			initialCall = false
			return true
		}
		return evm.CanTransfer(state, address, amount)
	}

	transfer := func(state evm.State, from, to common.Address, amount *big.Int) error {
		return nil
	}

	state, _ := buildState(t, c.Pre)
	txn := state.Txn()

	e := evm.NewEVM(txn, env, mainnetChainConfig.Forks.At(env.Number.Uint64()), chain.GasTableHomestead, vmTestBlockHash)
	e.CanTransfer = canTransfer
	e.Transfer = transfer

	ret, gas, err := e.Call(c.Exec.Caller, c.Exec.Address, c.Exec.Data, c.Exec.Value, c.Exec.GasLimit)

	if c.Gas == "" {
		if err == nil {
			t.Fatalf("gas unspecified (indicating an error), but VM returned no error")
		}
		if gas > 0 {
			t.Fatalf("gas unspecified (indicating an error), but VM returned gas remaining > 0")
		}
		return
	}

	// check return
	if c.Out == "" {
		c.Out = "0x"
	}
	if ret := hexutil.Encode(ret); ret != c.Out {
		t.Fatalf("return mismatch: got %s, want %s", ret, c.Out)
	}

	// check logs
	if logs := rlpHash(txn.Logs()); logs != common.HexToHash(c.Logs) {
		t.Fatalf("logs hash mismatch: got %x, want %x", logs, c.Logs)
	}

	// check state
	for addr, alloc := range c.Post {
		for key, val := range alloc.Storage {
			if have := txn.GetState(addr, key); have != val {
				t.Fatalf("wrong storage value at %x:\n  got  %x\n  want %x", key, have, val)
			}
		}
	}

	// check remaining gas
	if expected := stringToUint64T(t, c.Gas); gas != expected {
		t.Fatalf("gas remaining mismatch: got %d want %d", gas, expected)
	}
}

func TestEVM(t *testing.T) {
	folders, err := listFolders(vmTests)
	if err != nil {
		t.Fatal(err)
	}

	long := []string{
		"loop-",
	}

	for _, folder := range folders {
		files, err := listFiles(folder)
		if err != nil {
			t.Fatal(err)
		}

		for _, file := range files {
			t.Run(file, func(t *testing.T) {
				if !strings.HasSuffix(file, ".json") {
					return
				}

				data, err := ioutil.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}

				var vmcases map[string]*VMCase
				if err := json.Unmarshal(data, &vmcases); err != nil {
					t.Fatal(err)
				}

				for name, cc := range vmcases {
					if contains(long, name) && testing.Short() {
						t.Skip()
						continue
					}

					testVMCase(t, name, cc)
				}
			})
		}
	}
}

func vmTestBlockHash(n uint64) common.Hash {
	return common.BytesToHash(crypto.Keccak256([]byte(big.NewInt(int64(n)).String())))
}
