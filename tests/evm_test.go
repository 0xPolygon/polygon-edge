package tests

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/umbracle/minimal/evm"
)

type VMCase struct {
	Info *info `json:"_info"`
	Env  *env  `json:"env"`
	Exec *exec `json:"exec"`

	Gas  string `json:"gas"`
	Logs string `json:"logs"`
	Out  string `json:"out"`

	Post stateSnapshop `json:"post"`
	Pre  stateSnapshop `json:"pre"`
}

func testVMCase(name string, t *testing.T, c *VMCase) {
	env := c.Env.ToEnv(t)
	env.GasPrice = c.Exec.GasPrice

	state := buildState(t, c.Pre)

	e := evm.NewEVM(state, env, nil, params.GasTableHomestead, vmTestBlockHash)

	fmt.Printf("BlockNumber: %s\n", c.Env.Number)

	ret, gas, err := e.Call(c.Exec.Caller, c.Exec.Address, c.Exec.Data, c.Exec.Value, c.Exec.GasLimit)

	fmt.Println(name)
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
	if logs := rlpHash(state.Logs()); logs != common.HexToHash(c.Logs) {
		t.Fatalf("logs hash mismatch: got %x, want %x", logs, c.Logs)
	}

	// check state
	for i, account := range c.Post {
		addr := stringToAddressT(t, i)

		for k, v := range account.Storage {
			key := common.HexToHash(k)
			val := common.HexToHash(v)

			if have := state.GetState(addr, key); have != val {
				t.Fatalf("wrong storage value at %x:\n  got  %x\n  want %x", k, have, val)
			}
		}
	}

	// check remaining gas
	if expected := stringToUint64T(t, c.Gas); gas != expected {
		t.Fatalf("gas remaining mismatch: got %d want %d", gas, expected)
	}
}

var skip = []string{}

// not sure yet how to catch the bad cases

func skipTest(name string) bool {
	for _, i := range skip {
		if name == i {
			return true
		}
	}
	return false
}

func TestOne(t *testing.T) {
	files, err := listFiles("VMTests")
	if err != nil {
		panic(err)
	}

	count := len(files)
	for indx, file := range files {
		fmt.Printf("%d|%d: %s\n", indx, count, file)

		data, err := ioutil.ReadFile(file)
		if err != nil {
			panic(err)
		}

		var vmcases map[string]*VMCase
		if err := json.Unmarshal(data, &vmcases); err != nil {
			panic(err)
		}

		for name, cc := range vmcases {
			// fmt.Println(cc)

			// skip them for now
			if strings.HasPrefix(name, "loop-") {
				continue
			}
			if skipTest(name) {
				continue
			}

			testVMCase(name, t, cc)
		}
	}
}

func listFiles(folder string) ([]string, error) {
	files := []string{}
	err := filepath.Walk(filepath.Join(TESTS, folder), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func vmTestBlockHash(n uint64) common.Hash {
	return common.BytesToHash(crypto.Keccak256([]byte(big.NewInt(int64(n)).String())))
}
