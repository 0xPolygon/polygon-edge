package tests

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbracle/fastrlp"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/0xPolygon/polygon-edge/state"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
)

type testCase struct {
	Env         *env                                    `json:"env"`
	Pre         map[types.Address]*chain.GenesisAccount `json:"pre"`
	Post        map[string]postState                    `json:"post"`
	Transaction *stTransaction                          `json:"transaction"`
}

type env struct {
	BaseFee    string `json:"currentBaseFee"`
	Coinbase   string `json:"currentCoinbase"`
	Difficulty string `json:"currentDifficulty"`
	GasLimit   string `json:"currentGasLimit"`
	Number     string `json:"currentNumber"`
	Timestamp  string `json:"currentTimestamp"`
}

func remove0xPrefix(str string) string {
	if strings.HasPrefix(str, "0x") {
		return strings.Replace(str, "0x", "", -1)
	}

	return str
}

func stringToAddress(str string) (types.Address, error) {
	if str == "" {
		return types.Address{}, fmt.Errorf("value not found")
	}

	return types.StringToAddress(str), nil
}

func stringToHash(str string) (types.Hash, error) {
	if str == "" {
		return types.Hash{}, fmt.Errorf("value not found")
	}

	return types.StringToHash(str), nil
}

func stringToBigInt(str string) (*big.Int, error) {
	if str == "" {
		return nil, fmt.Errorf("value not found")
	}

	base := 10

	if strings.HasPrefix(str, "0x") {
		str, base = remove0xPrefix(str), 16
	}

	n, ok := big.NewInt(1).SetString(str, base)

	if !ok {
		return nil, fmt.Errorf("failed to convert %s to big.Int with base %d", str, base)
	}

	return n, nil
}

func stringToBigIntT(t *testing.T, str string) *big.Int {
	t.Helper()

	number, err := stringToBigInt(str)
	if err != nil {
		t.Fatal(err)
	}

	return number
}

func stringToAddressT(t *testing.T, str string) types.Address {
	t.Helper()

	address, err := stringToAddress(str)
	if err != nil {
		t.Fatal(err)
	}

	return address
}

func stringToHashT(t *testing.T, str string) types.Hash {
	t.Helper()

	address, err := stringToHash(str)
	if err != nil {
		t.Fatal(err)
	}

	return address
}

func stringToUint64(str string) (uint64, error) {
	n, err := stringToBigInt(str)
	if err != nil {
		return 0, err
	}

	return n.Uint64(), nil
}

func stringToUint64T(t *testing.T, str string) uint64 {
	t.Helper()

	n, err := stringToUint64(str)
	if err != nil {
		t.Fatal(err)
	}

	return n
}

func stringToInt64T(t *testing.T, str string) int64 {
	t.Helper()

	n, err := stringToUint64(str)
	if err != nil {
		t.Fatal(err)
	}

	return int64(n)
}

func (e *env) ToHeader(t *testing.T) *types.Header {
	t.Helper()

	baseFee := uint64(0)
	if e.BaseFee != "" {
		baseFee = stringToUint64T(t, e.BaseFee)
	}

	return &types.Header{
		Miner:      stringToAddressT(t, e.Coinbase).Bytes(),
		BaseFee:    baseFee,
		Difficulty: stringToUint64T(t, e.Difficulty),
		GasLimit:   stringToUint64T(t, e.GasLimit),
		Number:     stringToUint64T(t, e.Number),
		Timestamp:  stringToUint64T(t, e.Timestamp),
	}
}

func (e *env) ToEnv(t *testing.T) runtime.TxContext {
	t.Helper()

	baseFee := new(big.Int)
	if e.BaseFee != "" {
		baseFee = stringToBigIntT(t, e.BaseFee)
	}

	return runtime.TxContext{
		Coinbase:   stringToAddressT(t, e.Coinbase),
		BaseFee:    baseFee,
		Difficulty: stringToHashT(t, e.Difficulty),
		GasLimit:   stringToInt64T(t, e.GasLimit),
		Number:     stringToInt64T(t, e.Number),
		Timestamp:  stringToInt64T(t, e.Timestamp),
	}
}

func buildState(
	allocs map[types.Address]*chain.GenesisAccount,
) (state.State, state.Snapshot, types.Hash, error) {
	s := itrie.NewState(itrie.NewMemoryStorage())
	snap := s.NewSnapshot()

	txn := state.NewTxn(snap)

	for addr, alloc := range allocs {
		txn.CreateAccount(addr)
		txn.SetNonce(addr, alloc.Nonce)
		txn.SetBalance(addr, alloc.Balance)

		if len(alloc.Code) != 0 {
			txn.SetCode(addr, alloc.Code)
		}

		for k, v := range alloc.Storage {
			txn.SetState(addr, k, v)
		}
	}

	objs, err := txn.Commit(false)
	if err != nil {
		return nil, nil, types.ZeroHash, err
	}

	snap, root, err := snap.Commit(objs)

	return s, snap, types.BytesToHash(root), err
}

type indexes struct {
	Data  int `json:"data"`
	Gas   int `json:"gas"`
	Value int `json:"value"`
}

type postEntry struct {
	Root    types.Hash
	Logs    types.Hash
	Indexes indexes
	TxBytes []byte
}

type postState []postEntry

func (p *postEntry) UnmarshalJSON(input []byte) error {
	type stateUnmarshall struct {
		Root    string  `json:"hash"`
		Logs    string  `json:"logs"`
		Indexes indexes `json:"indexes"`
		TxBytes string  `json:"txbytes"`
	}

	var dec stateUnmarshall
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}

	p.Root = types.StringToHash(dec.Root)
	p.Logs = types.StringToHash(dec.Logs)
	p.Indexes = dec.Indexes
	p.TxBytes = types.StringToBytes(dec.TxBytes)

	return nil
}

type stTransaction struct {
	Data                 []string       `json:"data"`
	GasLimit             []uint64       `json:"gasLimit"`
	Value                []*big.Int     `json:"value"`
	GasPrice             *big.Int       `json:"gasPrice"`
	MaxFeePerGas         *big.Int       `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *big.Int       `json:"maxPriorityFeePerGas"`
	Nonce                uint64         `json:"nonce"`
	From                 types.Address  `json:"secretKey"`
	To                   *types.Address `json:"to"`
}

func (t *stTransaction) At(i indexes, baseFee *big.Int) (*types.Transaction, error) {
	if i.Data > len(t.Data) {
		return nil, fmt.Errorf("data index %d out of bounds (%d)", i.Data, len(t.Data))
	}

	if i.Gas > len(t.GasLimit) {
		return nil, fmt.Errorf("gas index %d out of bounds (%d)", i.Gas, len(t.GasLimit))
	}

	if i.Value > len(t.Value) {
		return nil, fmt.Errorf("value index %d out of bounds (%d)", i.Value, len(t.Value))
	}

	gasPrice := t.GasPrice

	// If baseFee provided, set gasPrice to effectiveGasPrice.
	if baseFee != nil {
		if t.MaxFeePerGas == nil {
			t.MaxFeePerGas = gasPrice
		}

		if t.MaxFeePerGas == nil {
			t.MaxFeePerGas = new(big.Int)
		}

		if t.MaxPriorityFeePerGas == nil {
			t.MaxPriorityFeePerGas = t.MaxFeePerGas
		}

		gasPrice = common.BigMin(new(big.Int).Add(t.MaxPriorityFeePerGas, baseFee), t.MaxFeePerGas)
	}

	return &types.Transaction{
		From:      t.From,
		To:        t.To,
		Nonce:     t.Nonce,
		Value:     new(big.Int).Set(t.Value[i.Value]),
		Gas:       t.GasLimit[i.Gas],
		GasPrice:  new(big.Int).Set(gasPrice),
		GasFeeCap: t.MaxFeePerGas,
		GasTipCap: t.MaxPriorityFeePerGas,
		Input:     hex.MustDecodeHex(t.Data[i.Data]),
	}, nil
}

func (t *stTransaction) UnmarshalJSON(input []byte) error {
	type txUnmarshall struct {
		Data                 []string `json:"data,omitempty"`
		GasLimit             []string `json:"gasLimit,omitempty"`
		Value                []string `json:"value,omitempty"`
		GasPrice             string   `json:"gasPrice,omitempty"`
		MaxFeePerGas         string   `json:"maxFeePerGas,omitempty"`
		MaxPriorityFeePerGas string   `json:"maxPriorityFeePerGas,omitempty"`
		Nonce                string   `json:"nonce,omitempty"`
		SecretKey            string   `json:"secretKey,omitempty"`
		To                   string   `json:"to,omitempty"`
	}

	var dec txUnmarshall
	if err := json.Unmarshal(input, &dec); err != nil {
		return fmt.Errorf("failed to unmarshal transaction into temporary struct: %w", err)
	}

	t.Data = dec.Data

	for _, i := range dec.GasLimit {
		j, err := stringToUint64(i)
		if err != nil {
			return fmt.Errorf("failed to convert string '%s' to uint64: %w", i, err)
		}

		t.GasLimit = append(t.GasLimit, j)
	}

	for _, i := range dec.Value {
		value := new(big.Int)
		loopVal := i

		if loopVal != "0x" {
			v, err := common.ParseUint256orHex(&loopVal)
			if err != nil {
				return err
			}

			value = v
		}

		t.Value = append(t.Value, value)
	}

	var err error

	if dec.GasPrice != "" {
		if t.GasPrice, err = stringToBigInt(dec.GasPrice); err != nil {
			return fmt.Errorf("failed to parse gas price: %w", err)
		}
	}

	if dec.MaxFeePerGas != "" {
		if t.MaxFeePerGas, err = stringToBigInt(dec.MaxFeePerGas); err != nil {
			return fmt.Errorf("failed to parse max fee per gas: %w", err)
		}
	}

	if dec.MaxPriorityFeePerGas != "" {
		if t.MaxPriorityFeePerGas, err = stringToBigInt(dec.MaxPriorityFeePerGas); err != nil {
			return fmt.Errorf("failed to parse max priority fee per gas: %w", err)
		}
	}

	if dec.Nonce != "" {
		if t.Nonce, err = stringToUint64(dec.Nonce); err != nil {
			return fmt.Errorf("failed to parse nonce: %w", err)
		}
	}

	t.From = types.Address{}

	if len(dec.SecretKey) > 0 {
		secretKey, err := common.ParseBytes(&dec.SecretKey)
		if err != nil {
			return fmt.Errorf("failed to parse secret key: %w", err)
		}

		key, err := crypto.ParseECDSAPrivateKey(secretKey)
		if err != nil {
			return fmt.Errorf("invalid private key: %w", err)
		}

		t.From = crypto.PubKeyToAddress(&key.PublicKey)
	}

	if dec.To != "" {
		address := types.StringToAddress(dec.To)
		t.To = &address
	}

	return nil
}

// forks

var Forks = map[string]*chain.Forks{
	"Frontier": {},
	"Homestead": {
		chain.Homestead: chain.NewFork(0),
	},
	"EIP150": {
		chain.Homestead: chain.NewFork(0),
		chain.EIP150:    chain.NewFork(0),
	},
	"EIP158": {
		chain.Homestead: chain.NewFork(0),
		chain.EIP150:    chain.NewFork(0),
		chain.EIP155:    chain.NewFork(0),
		chain.EIP158:    chain.NewFork(0),
	},
	"Byzantium": {
		chain.Homestead: chain.NewFork(0),
		chain.EIP150:    chain.NewFork(0),
		chain.EIP155:    chain.NewFork(0),
		chain.EIP158:    chain.NewFork(0),
		chain.Byzantium: chain.NewFork(0),
	},
	"Constantinople": {
		chain.Homestead:      chain.NewFork(0),
		chain.EIP150:         chain.NewFork(0),
		chain.EIP155:         chain.NewFork(0),
		chain.EIP158:         chain.NewFork(0),
		chain.Byzantium:      chain.NewFork(0),
		chain.Constantinople: chain.NewFork(0),
	},
	"Istchain.anbul": {
		chain.Homestead:      chain.NewFork(0),
		chain.EIP150:         chain.NewFork(0),
		chain.EIP155:         chain.NewFork(0),
		chain.EIP158:         chain.NewFork(0),
		chain.Byzantium:      chain.NewFork(0),
		chain.Constantinople: chain.NewFork(0),
		chain.Petersburg:     chain.NewFork(0),
		chain.Istanbul:       chain.NewFork(0),
	},
	"FrontierToHomesteadAt5": {
		chain.Homestead: chain.NewFork(5),
	},
	"HomesteadToEIP150At5": {
		chain.Homestead: chain.NewFork(0),
		chain.EIP150:    chain.NewFork(5),
	},
	"HomesteadToDaoAt5": {
		chain.Homestead: chain.NewFork(0),
	},
	"EIP158ToByzantiumAt5": {
		chain.Homestead: chain.NewFork(0),
		chain.EIP150:    chain.NewFork(0),
		chain.EIP155:    chain.NewFork(0),
		chain.EIP158:    chain.NewFork(0),
		chain.Byzantium: chain.NewFork(5),
	},
	"ByzantiumToConstantinopleAt5": {
		chain.Byzantium:      chain.NewFork(0),
		chain.Constantinople: chain.NewFork(5),
	},
	"ConstantinopleFix": {
		chain.Homestead:      chain.NewFork(0),
		chain.EIP150:         chain.NewFork(0),
		chain.EIP155:         chain.NewFork(0),
		chain.EIP158:         chain.NewFork(0),
		chain.Byzantium:      chain.NewFork(0),
		chain.Constantinople: chain.NewFork(0),
		chain.Petersburg:     chain.NewFork(0),
	},
}

func contains(l []string, name string) bool {
	for _, i := range l {
		if strings.Contains(name, i) {
			return true
		}
	}

	return false
}

//go:embed tests
var testsFS embed.FS

func listFolders(tests ...string) ([]string, error) {
	var folders []string

	for _, t := range tests {
		if err := fs.WalkDir(testsFS, t, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() && t != "path" {
				folders = append(folders, path)
			}

			return nil
		}); err != nil {
			return nil, err
		}

		// Excluding root dir
		folders = folders[1:]
	}

	return folders, nil
}

func listFiles(folder string) ([]string, error) {
	var files []string

	err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
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

func rlpHashLogs(logs []*types.Log) (res types.Hash) {
	r := &types.Receipt{
		Logs: logs,
	}

	ar := &fastrlp.Arena{}
	v := r.MarshalLogsWith(ar)

	keccak.Keccak256Rlp(res[:0], v)

	return
}

func vmTestBlockHash(n uint64) types.Hash {
	return types.BytesToHash(crypto.Keccak256([]byte(big.NewInt(int64(n)).String())))
}
