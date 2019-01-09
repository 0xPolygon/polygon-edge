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

	"github.com/umbracle/minimal/chain"

	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/ethereum/go-ethereum/common"

	"github.com/umbracle/minimal/state/evm"
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

type stateEntry struct {
	Balance string            `json:"balance"`
	Code    string            `json:"code"`
	Nonce   string            `json:"nonce"`
	Storage map[string]string `json:"storage"`
}

type stateSnapshop map[string]*stateEntry

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

func stringToAddress(str string) (common.Address, error) {
	if str == "" {
		return common.Address{}, fmt.Errorf("value not found")
	}
	return common.HexToAddress(str), nil
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

func stringToAddressT(t *testing.T, str string) common.Address {
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

func stringToBytesT(t *testing.T, str string) []byte {
	data, err := hexutil.Decode(str)
	if err != nil {
		panic(err)
		t.Fatal(err)
	}
	return data
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

func (e *env) ToEnv(t *testing.T) *evm.Env {
	return &evm.Env{
		BlockHash:  common.HexToHash(""),
		Coinbase:   stringToAddressT(t, e.Coinbase),
		Difficulty: stringToBigIntT(t, e.Difficulty),
		GasLimit:   stringToBigIntT(t, e.GasLimit),
		Number:     stringToBigIntT(t, e.Number),
		Timestamp:  stringToBigIntT(t, e.Timestamp),
	}
}

type exec struct {
	Address  common.Address
	Caller   common.Address
	Origin   common.Address
	Code     []byte
	Data     []byte
	Value    *big.Int
	GasLimit uint64
	GasPrice *big.Int
}

func (e *exec) UnmarshalJSON(input []byte) error {
	type execUnmarshall struct {
		Address  string `json:"address"`
		Caller   string `json:"caller"`
		Code     string `json:"code"`
		Data     string `json:"data"`
		Gas      string `json:"gas"`
		GasPrice string `json:"gasPrice"`
		Origin   string `json:"origin"`
		Value    string `json:"value"`
	}

	var dec execUnmarshall
	err := json.Unmarshal(input, &dec)
	if err != nil {
		return err
	}

	e.Address, err = stringToAddress(dec.Address)
	if err != nil {
		return err
	}
	e.Caller, err = stringToAddress(dec.Caller)
	if err != nil {
		return err
	}
	e.Code, err = hexutil.Decode(dec.Code)
	if err != nil {
		return err
	}
	e.Data, err = hexutil.Decode(dec.Data)
	if err != nil {
		return err
	}
	e.GasLimit, err = stringToUint64(dec.Gas)
	if err != nil {
		return err
	}
	e.GasPrice, err = stringToBigInt(dec.GasPrice)
	if err != nil {
		return err
	}
	e.Origin, err = stringToAddress(dec.Origin)
	if err != nil {
		return err
	}
	e.Value, err = stringToBigInt(dec.Value)
	if err != nil {
		return err
	}
	return nil
}

func buildState(t *testing.T, pre stateSnapshop) *state.StateDB {
	db := state.NewDatabase(ethdb.NewMemDatabase())
	statedb, err := state.New(common.Hash{}, db)
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range pre {
		addr := stringToAddressT(t, i)

		statedb.CreateAccount(addr)
		if a.Code != "" {
			statedb.SetCode(addr, stringToBytesT(t, a.Code))
		}
		statedb.SetNonce(addr, stringToUint64T(t, a.Nonce))
		statedb.SetBalance(addr, stringToBigIntT(t, a.Balance))

		for k, v := range a.Storage {
			statedb.SetState(addr, common.HexToHash(k), common.HexToHash(v))
		}
	}
	root, err := statedb.Commit(false)
	if err != nil {
		t.Fatalf("failed to commit pre state: %v", err)
	}
	statedb, err = state.New(root, db)
	if err != nil {
		t.Fatal(err)
	}
	return statedb
}

func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
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
	Root    common.Hash
	Logs    common.Hash
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

	p.Root = common.HexToHash(dec.Root)
	p.Logs = common.HexToHash(dec.Logs)
	p.Indexes = dec.Indexes

	return nil
}

type stTransaction struct {
	Data     []string        `json:"data"`
	GasLimit []uint64        `json:"gasLimit"`
	Value    []*big.Int      `json:"value"`
	GasPrice *big.Int        `json:"gasPrice"`
	Nonce    uint64          `json:"nonce"`
	From     common.Address  `json:"secretKey"`
	To       *common.Address `json:"to"`
}

func (t *stTransaction) At(i indexes) (*types.Message, error) {
	if i.Data > len(t.Data) {
		return nil, fmt.Errorf("data index %d out of bounds (%d)", i.Data, len(t.Data))
	}
	if i.Gas > len(t.GasLimit) {
		return nil, fmt.Errorf("gas index %d out of bounds (%d)", i.Gas, len(t.GasLimit))
	}
	if i.Value > len(t.Value) {
		return nil, fmt.Errorf("value index %d out of bounds (%d)", i.Value, len(t.Value))
	}

	msg := types.NewMessage(t.From, t.To, t.Nonce, t.Value[i.Value], t.GasLimit[i.Gas], t.GasPrice, hexutil.MustDecode(t.Data[i.Data]), true)
	return &msg, nil
}

func (t *stTransaction) UnmarshalJSON(input []byte) error {
	type txUnmarshall struct {
		Data      []string      `json:"data"`
		GasLimit  []string      `json:"gasLimit"`
		Value     []string      `json:"value"`
		GasPrice  string        `json:"gasPrice"`
		Nonce     string        `json:"nonce"`
		SecretKey hexutil.Bytes `json:"secretKey"`
		To        string        `json:"to"`
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
			v, ok := math.ParseBig256(i)
			if !ok {
				return fmt.Errorf("invalid tx value %q", i)
			}
			value = v
		}
		t.Value = append(t.Value, value)
	}

	t.GasPrice, err = stringToBigInt(dec.GasPrice)
	t.Nonce, err = stringToUint64(dec.Nonce)
	if err != nil {
		return err
	}

	t.From = common.Address{}
	if len(dec.SecretKey) > 0 {
		key, err := crypto.ToECDSA(dec.SecretKey)
		if err != nil {
			return fmt.Errorf("invalid private key: %v", err)
		}
		t.From = crypto.PubkeyToAddress(key.PublicKey)
	}

	if dec.To != "" {
		address := common.HexToAddress(dec.To)
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
		EIP158:    chain.NewFork(0),
	},
	"Byzantium": {
		Homestead: chain.NewFork(0),
		EIP150:    chain.NewFork(0),
		EIP158:    chain.NewFork(0),
		Byzantium: chain.NewFork(0),
	},
	"Constantinople": {
		Homestead:      chain.NewFork(0),
		EIP150:         chain.NewFork(0),
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
		EIP158:    chain.NewFork(0),
		Byzantium: chain.NewFork(5),
	},
}

type header struct {
	header *types.Header
}

func (h *header) UnmarshalJSON(input []byte) error {
	h.header = &types.Header{}

	type headerUnmarshall struct {
		Bloom            *types.Bloom
		Coinbase         *common.Address
		MixHash          *common.Hash
		Nonce            *types.BlockNonce
		Number           *math.HexOrDecimal256
		Hash             *common.Hash
		ParentHash       *common.Hash
		ReceiptTrie      *common.Hash
		StateRoot        *common.Hash
		TransactionsTrie *common.Hash
		UncleHash        *common.Hash
		ExtraData        *hexutil.Bytes
		Difficulty       *math.HexOrDecimal256
		GasLimit         *math.HexOrDecimal64
		GasUsed          *math.HexOrDecimal64
		Timestamp        *math.HexOrDecimal256
	}

	var dec headerUnmarshall
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}

	if dec.Bloom != nil {
		h.header.Bloom = *dec.Bloom
	}
	if dec.Coinbase != nil {
		h.header.Coinbase = *dec.Coinbase
	}
	if dec.MixHash != nil {
		h.header.MixDigest = *dec.MixHash
	}
	if dec.Nonce != nil {
		h.header.Nonce = *dec.Nonce
	}
	if dec.Number != nil {
		h.header.Number = (*big.Int)(dec.Number)
	}
	if dec.ParentHash != nil {
		h.header.ParentHash = *dec.ParentHash
	}
	if dec.ReceiptTrie != nil {
		h.header.ReceiptHash = *dec.ReceiptTrie
	}
	if dec.StateRoot != nil {
		h.header.Root = *dec.StateRoot
	}
	if dec.TransactionsTrie != nil {
		h.header.TxHash = *dec.TransactionsTrie
	}
	if dec.UncleHash != nil {
		h.header.UncleHash = *dec.UncleHash
	}
	if dec.ExtraData != nil {
		h.header.Extra = *dec.ExtraData
	}
	if dec.Difficulty != nil {
		h.header.Difficulty = (*big.Int)(dec.Difficulty)
	}
	if dec.GasLimit != nil {
		h.header.GasLimit = uint64(*dec.GasLimit)
	}
	if dec.GasUsed != nil {
		h.header.GasUsed = uint64(*dec.GasUsed)
	}
	if dec.Timestamp != nil {
		h.header.Time = (*big.Int)(dec.Timestamp)
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
	if strings.HasPrefix(TESTS, folder) {
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
