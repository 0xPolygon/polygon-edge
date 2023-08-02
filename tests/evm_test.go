package tests

import (
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/state/runtime/evm"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/fastrlp"
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

	Post map[types.Address]*chain.GenesisAccount `json:"post"`
	Pre  map[types.Address]*chain.GenesisAccount `json:"pre"`
}

func testVMCase(t *testing.T, name string, c *VMCase) {
	t.Helper()

	env := c.Env.ToEnv(t)
	env.GasPrice = types.BytesToHash(c.Exec.GasPrice.Bytes())
	env.Origin = c.Exec.Origin

	s, _, root := buildState(c.Pre)

	config := mainnetChainConfig.Forks.At(uint64(env.Number))

	executor := state.NewExecutor(&mainnetChainConfig, s, hclog.NewNullLogger())
	executor.GetHash = func(*types.Header) func(i uint64) types.Hash {
		return vmTestBlockHash
	}

	e, _ := executor.BeginTxn(root, c.Env.ToHeader(t), env.Coinbase)
	ctx := e.ContextPtr()
	ctx.GasPrice = types.BytesToHash(env.GasPrice.Bytes())
	ctx.Origin = env.Origin

	evmR := evm.NewEVM()

	code := e.GetCode(c.Exec.Address)
	contract := runtime.NewContractCall(
		1,
		c.Exec.Caller,
		c.Exec.Caller,
		c.Exec.Address,
		c.Exec.Value,
		c.Exec.GasLimit,
		code,
		c.Exec.Data,
	)

	result := evmR.Run(contract, e, &config)

	if c.Gas == "" {
		if result.Succeeded() {
			t.Fatalf("gas unspecified (indicating an error), but VM returned no error")
		}

		if result.GasLeft > 0 {
			t.Fatalf("gas unspecified (indicating an error), but VM returned gas remaining > 0")
		}

		return
	}

	// check return
	if c.Out == "" {
		c.Out = "0x"
	}

	if ret := hex.EncodeToHex(result.ReturnValue); ret != c.Out {
		t.Fatalf("return mismatch: got %s, want %s", ret, c.Out)
	}

	txn := e.Txn()

	// check logs
	if logs := rlpHashLogs(txn.Logs()); logs != types.StringToHash(c.Logs) {
		t.Fatalf("logs hash mismatch: got %x, want %x", logs, c.Logs)
	}

	// check state
	for addr, alloc := range c.Post {
		for key, val := range alloc.Storage {
			if have := txn.GetState(addr, key); have != val {
				t.Fatalf("wrong storage value at %s:\n  got  %s\n  want %s\n at address %s", key, have, val, addr)
			}
		}
	}

	// check remaining gas
	if expected := stringToUint64T(t, c.Gas); result.GasLeft != expected {
		t.Fatalf("gas left mismatch: got %d want %d", result.GasLeft, expected)
	}
}

func rlpHashLogs(logs []*types.Log) (res types.Hash) {
	r := &types.Receipt{
		Logs: logs,
	}

	ar := &fastrlp.Arena{}
	v := r.MarshalLogsWith(ar)

	keccak.Keccak256Rlp(res[:0], v)

	return
}

func TestEVM(t *testing.T) {
	t.Parallel()

	folders, err := listFolders(vmTests)
	if err != nil {
		t.Fatal(err)
	}

	long := []string{
		"loop-",
		"gasprice",
		"origin",
	}

	for _, folder := range folders {
		files, err := listFiles(folder)
		if err != nil {
			t.Fatal(err)
		}

		for _, file := range files {
			file := file
			t.Run(file, func(t *testing.T) {
				t.Parallel()

				if !strings.HasSuffix(file, ".json") {
					return
				}

				data, err := os.ReadFile(file)
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

func vmTestBlockHash(n uint64) types.Hash {
	return types.BytesToHash(crypto.Keccak256([]byte(big.NewInt(int64(n)).String())))
}
