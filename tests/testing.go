package tests

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbracle/minimal/crypto"
	"github.com/umbracle/minimal/helper/hex"
	trie "github.com/umbracle/minimal/state/immutable-trie"
	"github.com/umbracle/minimal/state/runtime"
	"github.com/umbracle/minimal/state/runtime/precompiled"

	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/state"

	"github.com/umbracle/minimal/rlp"
	"github.com/umbracle/minimal/types"
	"golang.org/x/crypto/sha3"
)

// TESTS is the default location of the tests folder
const TESTS = "./tests"

type info struct {
	Comment     string `json:"comment"`
	FilledWith  string `json:"filledwith"`
	LllcVersion string `json:"lllcversion"`
	Source      string `json:"source"`
	SourceHash  string `json:"sourcehash"`
}

type env struct {
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

func stringToAddressT(t *testing.T, str string) types.Address {
	address, err := stringToAddress(str)
	if err != nil {
		t.Fatal(err)
	}
	return address
}

func stringToBigIntT(t *testing.T, str string) *big.Int {
	n, err := stringToBigInt(str)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func stringToUint64(str string) (uint64, error) {
	n, err := stringToBigInt(str)
	if err != nil {
		return 0, err
	}
	return n.Uint64(), nil
}

func stringToUint64T(t *testing.T, str string) uint64 {
	n, err := stringToUint64(str)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *env) ToEnv(t *testing.T) *runtime.Env {
	return &runtime.Env{
		Coinbase:   stringToAddressT(t, e.Coinbase),
		Difficulty: stringToBigIntT(t, e.Difficulty),
		GasLimit:   stringToBigIntT(t, e.GasLimit),
		Number:     stringToUint64T(t, e.Number),
		Timestamp:  stringToUint64T(t, e.Timestamp),
	}
}

type exec struct {
	Address  types.Address
	Caller   types.Address
	Origin   types.Address
	Code     []byte
	Data     []byte
	Value    *big.Int
	GasLimit uint64
	GasPrice *big.Int
}

func (e *exec) UnmarshalJSON(input []byte) error {
	type execUnmarshall struct {
		Address  types.Address `json:"address"`
		Caller   types.Address `json:"caller"`
		Origin   types.Address `json:"origin"`
		Code     string        `json:"code"`
		Data     string        `json:"data"`
		Value    string        `json:"value"`
		Gas      string        `json:"gas"`
		GasPrice string        `json:"gasPrice"`
	}

	var dec execUnmarshall
	err := json.Unmarshal(input, &dec)
	if err != nil {
		return err
	}

	e.Address = dec.Address
	e.Caller = dec.Caller
	e.Origin = dec.Origin

	e.Code, err = types.ParseBytes(&dec.Code)
	if err != nil {
		return err
	}
	e.Data, err = types.ParseBytes(&dec.Data)
	if err != nil {
		return err
	}

	e.Value, err = types.ParseUint256orHex(&dec.Value)
	if err != nil {
		return err
	}
	e.GasLimit, err = types.ParseUint64orHex(&dec.Gas)
	if err != nil {
		return err
	}
	e.GasPrice, err = types.ParseUint256orHex(&dec.GasPrice)
	if err != nil {
		return err
	}

	return nil
}

func buildState(t *testing.T, allocs chain.GenesisAlloc) (state.State, state.Snapshot, []byte) {
	s := trie.NewState(trie.NewMemoryStorage())
	snap := s.NewSnapshot()

	txn := state.NewTxn(s, snap)

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

	snap, root := txn.Commit(false)
	return s, snap, root
}

func rlpHash(x interface{}) (h types.Hash) {
	hw := sha3.NewLegacyKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
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
}

type postState []postEntry

func (p *postEntry) UnmarshalJSON(input []byte) error {
	type stateUnmarshall struct {
		Root    string  `json:"hash"`
		Logs    string  `json:"logs"`
		Indexes indexes `json:"indexes"`
	}

	var dec stateUnmarshall
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}

	p.Root = types.StringToHash(dec.Root)
	p.Logs = types.StringToHash(dec.Logs)
	p.Indexes = dec.Indexes

	return nil
}

type stTransaction struct {
	Data     []string       `json:"data"`
	GasLimit []uint64       `json:"gasLimit"`
	Value    []*big.Int     `json:"value"`
	GasPrice *big.Int       `json:"gasPrice"`
	Nonce    uint64         `json:"nonce"`
	From     types.Address  `json:"secretKey"`
	To       *types.Address `json:"to"`
}

func (t *stTransaction) At(i indexes) (*types.Transaction, error) {
	if i.Data > len(t.Data) {
		return nil, fmt.Errorf("data index %d out of bounds (%d)", i.Data, len(t.Data))
	}
	if i.Gas > len(t.GasLimit) {
		return nil, fmt.Errorf("gas index %d out of bounds (%d)", i.Gas, len(t.GasLimit))
	}
	if i.Value > len(t.Value) {
		return nil, fmt.Errorf("value index %d out of bounds (%d)", i.Value, len(t.Value))
	}

	msg := &types.Transaction{
		To:       t.To,
		Nonce:    t.Nonce,
		Value:    t.Value[i.Value],
		Gas:      t.GasLimit[i.Gas],
		GasPrice: t.GasPrice,
		Input:    hex.MustDecodeHex(t.Data[i.Data]),
	}

	msg.SetFrom(t.From)
	return msg, nil
}

func (t *stTransaction) UnmarshalJSON(input []byte) error {
	type txUnmarshall struct {
		Data      []string `json:"data"`
		GasLimit  []string `json:"gasLimit"`
		Value     []string `json:"value"`
		GasPrice  string   `json:"gasPrice"`
		Nonce     string   `json:"nonce"`
		SecretKey string   `json:"secretKey"`
		To        string   `json:"to"`
	}

	var dec txUnmarshall
	err := json.Unmarshal(input, &dec)
	if err != nil {
		return err
	}

	t.Data = dec.Data
	for _, i := range dec.GasLimit {
		if j, err := stringToUint64(i); err != nil {
			return err
		} else {
			t.GasLimit = append(t.GasLimit, j)
		}
	}

	for _, i := range dec.Value {
		value := new(big.Int)
		if i != "0x" {
			v, err := types.ParseUint256orHex(&i)
			if err != nil {
				return err
			}
			/*
				v, ok := math.ParseBig256(i)
				if !ok {
					return fmt.Errorf("invalid tx value %q", i)
				}
			*/
			value = v
		}
		t.Value = append(t.Value, value)
	}

	t.GasPrice, err = stringToBigInt(dec.GasPrice)
	t.Nonce, err = stringToUint64(dec.Nonce)
	if err != nil {
		return err
	}

	t.From = types.Address{}
	if len(dec.SecretKey) > 0 {
		secretKey, err := types.ParseBytes(&dec.SecretKey)
		if err != nil {
			return err
		}
		key, err := crypto.ParsePrivateKey(secretKey)
		if err != nil {
			return fmt.Errorf("invalid private key: %v", err)
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
		Homestead: chain.NewFork(0),
	},
	"EIP150": {
		Homestead: chain.NewFork(0),
		EIP150:    chain.NewFork(0),
	},
	"EIP158": {
		Homestead: chain.NewFork(0),
		EIP150:    chain.NewFork(0),
		EIP155:    chain.NewFork(0),
		EIP158:    chain.NewFork(0),
	},
	"Byzantium": {
		Homestead: chain.NewFork(0),
		EIP150:    chain.NewFork(0),
		EIP155:    chain.NewFork(0),
		EIP158:    chain.NewFork(0),
		Byzantium: chain.NewFork(0),
	},
	"Constantinople": {
		Homestead:      chain.NewFork(0),
		EIP150:         chain.NewFork(0),
		EIP155:         chain.NewFork(0),
		EIP158:         chain.NewFork(0),
		Byzantium:      chain.NewFork(0),
		Constantinople: chain.NewFork(0),
	},
	"FrontierToHomesteadAt5": {
		Homestead: chain.NewFork(5),
	},
	"HomesteadToEIP150At5": {
		Homestead: chain.NewFork(0),
		EIP150:    chain.NewFork(5),
	},
	"HomesteadToDaoAt5": {
		Homestead: chain.NewFork(0),
	},
	"EIP158ToByzantiumAt5": {
		Homestead: chain.NewFork(0),
		EIP150:    chain.NewFork(0),
		EIP155:    chain.NewFork(0),
		EIP158:    chain.NewFork(0),
		Byzantium: chain.NewFork(5),
	},
	"ByzantiumToConstantinopleAt5": {
		Byzantium:      chain.NewFork(0),
		Constantinople: chain.NewFork(5),
	},
}

type header struct {
	header *types.Header
}

func (h *header) UnmarshalJSON(input []byte) error {
	h.header = &types.Header{}

	type headerUnmarshall struct {
		Bloom            *types.Bloom
		Coinbase         *types.Address
		MixHash          *types.Hash
		Nonce            *string
		Number           *string
		Hash             *types.Hash
		ParentHash       *types.Hash
		ReceiptTrie      *types.Hash
		StateRoot        *types.Hash
		TransactionsTrie *types.Hash
		UncleHash        *types.Hash
		ExtraData        *string
		Difficulty       *string
		GasLimit         *string
		GasUsed          *string
		Timestamp        *string
	}

	var err error

	var dec headerUnmarshall
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}

	if dec.Bloom != nil {
		h.header.LogsBloom = *dec.Bloom
	}
	if dec.Coinbase != nil {
		h.header.Miner = *dec.Coinbase
	}
	if dec.MixHash != nil {
		h.header.MixHash = *dec.MixHash
	}

	nonce, err := types.ParseUint64orHex(dec.Nonce)
	if err != nil {
		return err
	}
	if nonce != 0 {
		binary.BigEndian.PutUint64(h.header.Nonce[:], nonce)
	}

	h.header.Number, err = types.ParseUint64orHex(dec.Number)
	if err != nil {
		return err
	}

	if dec.ParentHash != nil {
		h.header.ParentHash = *dec.ParentHash
	}
	if dec.ReceiptTrie != nil {
		h.header.ReceiptsRoot = *dec.ReceiptTrie
	}
	if dec.StateRoot != nil {
		h.header.StateRoot = *dec.StateRoot
	}
	if dec.TransactionsTrie != nil {
		h.header.TxRoot = *dec.TransactionsTrie
	}
	if dec.UncleHash != nil {
		h.header.Sha3Uncles = *dec.UncleHash
	}

	h.header.ExtraData, err = types.ParseBytes(dec.ExtraData)
	if err != nil {
		return err
	}

	h.header.Difficulty, err = types.ParseUint256orHex(dec.Difficulty)
	if err != nil {
		return err
	}

	h.header.GasLimit, err = types.ParseUint64orHex(dec.GasLimit)
	if err != nil {
		return err
	}
	h.header.GasUsed, err = types.ParseUint64orHex(dec.GasUsed)
	if err != nil {
		return err
	}
	h.header.Timestamp, err = types.ParseUint64orHex(dec.Timestamp)
	if err != nil {
		return err
	}

	if dec.Hash != nil {
		if hash := h.header.Hash(); hash != *dec.Hash {
			return fmt.Errorf("hash mismatch: found %s but expected %s", hash.String(), (*dec.Hash).String())
		}
	}
	return nil
}

func contains(l []string, name string) bool {
	for _, i := range l {
		if strings.Contains(name, i) {
			return true
		}
	}
	return false
}

func listFolders(folder string) ([]string, error) {
	path := filepath.Join(TESTS, folder)

	files, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}

	folders := []string{}
	for _, i := range files {
		if i.IsDir() {
			folders = append(folders, filepath.Join(path, i.Name()))
		}
	}
	return folders, nil
}

func listFiles(folder string) ([]string, error) {
	if !strings.HasPrefix(folder, filepath.Base(TESTS)) {
		folder = filepath.Join(TESTS, folder)
	}

	files := []string{}
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

func init() {
	if err := json.Unmarshal([]byte(homesteadBuiltinsStr), &homesteadBuiltins); err != nil {
		panic(err)
	}
	if err := json.Unmarshal([]byte(byzantiumBuiltinsStr), &byzantiumBuiltins); err != nil {
		panic(err)
	}
}

func buildBuiltins(t *testing.T, forks *chain.Forks) map[types.Address]*precompiled.Precompiled {
	p := map[types.Address]*precompiled.Precompiled{}

	addBuiltin := func(active uint64, addr types.Address, b *chain.Builtin) {
		aux, err := precompiled.CreatePrecompiled(b)
		if err != nil {
			t.Fatal(err)
		}
		aux.ActiveAt = active
		p[addr] = aux
	}

	for addr, b := range homesteadBuiltins {
		addBuiltin(0, addr, b.Builtin)
	}

	if forks.Byzantium != nil {
		byzantiumFork := forks.Byzantium.Int().Uint64()
		for addr, b := range byzantiumBuiltins {
			addBuiltin(byzantiumFork, addr, b.Builtin)
		}
	}

	return p
}

var homesteadBuiltins map[types.Address]*chain.GenesisAccount

var homesteadBuiltinsStr = `{
	"0x0000000000000000000000000000000000000001": {
		"builtin": { "name": "ecrecover", "pricing": { "base": 3000 } } 
	},
	"0x0000000000000000000000000000000000000002": { 
		"builtin": { "name": "sha256", "pricing": { "base": 60, "word": 12 } } 
	},
	"0x0000000000000000000000000000000000000003": {
		"builtin": { "name": "ripemd160", "pricing": { "base": 600, "word": 120 } } 
	},
	"0x0000000000000000000000000000000000000004": {
		"builtin": { "name": "identity", "pricing": { "base": 15, "word": 3 } } 
	}
}`

var byzantiumBuiltins map[types.Address]*chain.GenesisAccount

var byzantiumBuiltinsStr = `{
	"0x0000000000000000000000000000000000000005": {
		"builtin": { "name": "modexp", "activate_at": 4370000, "pricing": { "divisor": 20 }}
	},
	"0x0000000000000000000000000000000000000006": {
		"builtin": { "name": "alt_bn128_add", "activate_at": 4370000, "pricing": { "base": 500 } }
	},
	"0x0000000000000000000000000000000000000007": {
		"builtin": { "name": "alt_bn128_mul", "activate_at": 4370000, "pricing": { "base": 40000 } }
	},
	"0x0000000000000000000000000000000000000008": {
		"builtin": { "name": "alt_bn128_pairing", "activate_at": 4370000, "pricing": { "base": 100000, "pair": 80000 } }
	}
}`
