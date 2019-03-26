package chain

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/gobuffalo/packr"
)

const (
	// GasLimitBoundDivisor is the bound divisor of the gas limit, used in update calculations.
	GasLimitBoundDivisor uint64 = 1024
	// MinGasLimit is the minimum the gas limit may ever be.
	MinGasLimit uint64 = 5000
	// MaximumExtraDataSize is the maximum size extra data may be after Genesis.
	MaximumExtraDataSize uint64 = 32
)

var (
	// GenesisGasLimit is the default gas limit of the Genesis block.
	GenesisGasLimit uint64 = 4712388
	// GenesisDifficulty is the default difficulty of the Genesis block.
	GenesisDifficulty = big.NewInt(131072)
)

var (
	box = packr.NewBox("./chains")
)

type Chain struct {
	Name      string    `json:"name"`
	Genesis   *Genesis  `json:"genesis"`
	Params    *Params   `json:"params"`
	Bootnodes Bootnodes `json:"bootnodes"`
}

type Bootnodes []string

// Genesis specifies the header fields, state of a genesis block
type Genesis struct {
	Nonce      uint64         `json:"nonce"`
	Timestamp  uint64         `json:"timestamp"`
	ExtraData  []byte         `json:"extraData,omitempty"`
	GasLimit   uint64         `json:"gasLimit"`
	Difficulty *big.Int       `json:"difficulty"`
	Mixhash    common.Hash    `json:"mixHash"`
	Coinbase   common.Address `json:"coinbase"`
	Alloc      GenesisAlloc   `json:"alloc,omitempty"`

	// Only for testing
	Number     uint64      `json:"number"`
	GasUsed    uint64      `json:"gasUsed"`
	ParentHash common.Hash `json:"parentHash"`
}

func (g *Genesis) ToBlock() *types.Header {
	head := &types.Header{
		Number:      new(big.Int).SetUint64(g.Number),
		Nonce:       types.EncodeNonce(g.Nonce),
		Time:        new(big.Int).SetUint64(g.Timestamp),
		ParentHash:  g.ParentHash,
		Extra:       g.ExtraData,
		GasLimit:    g.GasLimit,
		GasUsed:     g.GasUsed,
		Difficulty:  g.Difficulty,
		MixDigest:   g.Mixhash,
		Coinbase:    g.Coinbase,
		UncleHash:   types.EmptyUncleHash,
		ReceiptHash: types.EmptyRootHash,
		TxHash:      types.EmptyRootHash,
	}
	if g.GasLimit == 0 {
		head.GasLimit = GenesisGasLimit
	}
	if g.Difficulty == nil {
		head.Difficulty = GenesisDifficulty
	}
	return head
}

// Decoding

// MarshalJSON implements the json interface
func (g *Genesis) MarshalJSON() ([]byte, error) {
	type Genesis struct {
		Nonce      math.HexOrDecimal64                         `json:"nonce"`
		Timestamp  math.HexOrDecimal64                         `json:"timestamp"`
		ExtraData  hexutil.Bytes                               `json:"extraData"`
		GasLimit   math.HexOrDecimal64                         `json:"gasLimit"`
		Difficulty *math.HexOrDecimal256                       `json:"difficulty"`
		Mixhash    common.Hash                                 `json:"mixHash"`
		Coinbase   common.Address                              `json:"coinbase"`
		Alloc      map[common.UnprefixedAddress]GenesisAccount `json:"alloc"`
		Number     math.HexOrDecimal64                         `json:"number"`
		GasUsed    math.HexOrDecimal64                         `json:"gasUsed"`
		ParentHash common.Hash                                 `json:"parentHash"`
	}

	var enc Genesis
	enc.Nonce = math.HexOrDecimal64(g.Nonce)
	enc.Timestamp = math.HexOrDecimal64(g.Timestamp)
	enc.ExtraData = g.ExtraData
	enc.GasLimit = math.HexOrDecimal64(g.GasLimit)
	enc.Difficulty = (*math.HexOrDecimal256)(g.Difficulty)
	enc.Mixhash = g.Mixhash
	enc.Coinbase = g.Coinbase
	enc.Alloc = make(map[common.UnprefixedAddress]GenesisAccount, len(g.Alloc))
	if g.Alloc != nil {
		for k, v := range g.Alloc {
			enc.Alloc[common.UnprefixedAddress(k)] = v
		}
	}
	enc.Number = math.HexOrDecimal64(g.Number)
	enc.GasUsed = math.HexOrDecimal64(g.GasUsed)
	enc.ParentHash = g.ParentHash

	return json.Marshal(&enc)
}

// UnmarshalJSON implements the json interface
func (g *Genesis) UnmarshalJSON(data []byte) error {
	type Genesis struct {
		Nonce      *math.HexOrDecimal64                        `json:"nonce"`
		Timestamp  *math.HexOrDecimal64                        `json:"timestamp"`
		ExtraData  *hexutil.Bytes                              `json:"extraData"`
		GasLimit   *math.HexOrDecimal64                        `json:"gasLimit"`
		Difficulty *math.HexOrDecimal256                       `json:"difficulty"`
		Mixhash    *common.Hash                                `json:"mixHash"`
		Coinbase   *common.Address                             `json:"coinbase"`
		Alloc      map[common.UnprefixedAddress]GenesisAccount `json:"alloc"`
		Number     *math.HexOrDecimal64                        `json:"number"`
		GasUsed    *math.HexOrDecimal64                        `json:"gasUsed"`
		ParentHash *common.Hash                                `json:"parentHash"`
	}

	var dec Genesis
	if err := json.Unmarshal(data, &dec); err != nil {
		return err
	}
	if dec.Nonce != nil {
		g.Nonce = uint64(*dec.Nonce)
	}
	if dec.Timestamp != nil {
		g.Timestamp = uint64(*dec.Timestamp)
	}
	if dec.ExtraData != nil {
		g.ExtraData = *dec.ExtraData
	}
	if dec.GasLimit == nil {
		return errors.New("missing required field 'gasLimit' for Genesis")
	}
	g.GasLimit = uint64(*dec.GasLimit)
	if dec.Difficulty == nil {
		return errors.New("missing required field 'difficulty' for Genesis")
	}
	g.Difficulty = (*big.Int)(dec.Difficulty)
	if dec.Mixhash != nil {
		g.Mixhash = *dec.Mixhash
	}
	if dec.Coinbase != nil {
		g.Coinbase = *dec.Coinbase
	}
	if dec.Alloc != nil {
		g.Alloc = make(GenesisAlloc, len(dec.Alloc))
		for k, v := range dec.Alloc {
			g.Alloc[common.Address(k)] = v
		}
	}
	if dec.Number != nil {
		g.Number = uint64(*dec.Number)
	}
	if dec.GasUsed != nil {
		g.GasUsed = uint64(*dec.GasUsed)
	}
	if dec.ParentHash != nil {
		g.ParentHash = *dec.ParentHash
	}
	return nil
}

// Genesis alloc

// GenesisAccount is an account in the state of the genesis block.
type GenesisAccount struct {
	Code       []byte
	Storage    map[common.Hash]common.Hash
	Balance    *big.Int
	Nonce      uint64
	Builtin    *Builtin
	PrivateKey []byte // for tests
}

// Builtin is a precompiled contract
type Builtin struct {
	Name       string            `json:"name"`
	ActivateAt uint64            `json:"activate_at"`
	Pricing    map[string]uint64 `json:"pricing"`
}

// GenesisAlloc specifies the initial state that is part of the genesis block.
type GenesisAlloc map[common.Address]GenesisAccount

// Decoding

func (g *GenesisAccount) UnmarshalJSON(data []byte) error {
	type GenesisAccount struct {
		Code       *hexutil.Bytes              `json:"code,omitempty"`
		Storage    map[storageJSON]storageJSON `json:"storage,omitempty"`
		Balance    *math.HexOrDecimal256       `json:"balance"`
		Nonce      *math.HexOrDecimal64        `json:"nonce,omitempty"`
		Builtin    *Builtin                    `json:"builtin,omitempty"`
		PrivateKey *hexutil.Bytes              `json:"secretKey,omitempty"`
	}

	var dec GenesisAccount
	if err := json.Unmarshal(data, &dec); err != nil {
		return err
	}

	if dec.Code != nil {
		g.Code = *dec.Code
	}
	if dec.Storage != nil {
		g.Storage = make(map[common.Hash]common.Hash, len(dec.Storage))
		for k, v := range dec.Storage {
			g.Storage[common.Hash(k)] = common.Hash(v)
		}
	}
	g.Balance = (*big.Int)(dec.Balance)
	if dec.Nonce != nil {
		g.Nonce = uint64(*dec.Nonce)
	}
	if dec.Builtin != nil {
		g.Builtin = dec.Builtin
	}
	if dec.PrivateKey != nil {
		g.PrivateKey = *dec.PrivateKey
	}
	return nil
}

type storageJSON common.Hash

func (h *storageJSON) UnmarshalText(text []byte) error {
	text = bytes.TrimPrefix(text, []byte("0x"))
	if len(text) > 64 {
		return fmt.Errorf("too many hex characters in storage key/value %q", text)
	}

	offset := len(h) - len(text)/2 // pad on the left
	if _, err := hex.Decode(h[offset:], text); err != nil {
		return fmt.Errorf("invalid hex storage key/value %q", text)
	}
	return nil
}

// ImportFromName imports a chain from the precompiled json chains (i.e. foundation)
func ImportFromName(chain string) (*Chain, error) {
	data, err := box.Find(chain + ".json")
	if err != nil {
		return nil, err
	}
	return importChain(data)
}

// ImportFromFile imports a chain from a filepath
func ImportFromFile(filename string) (*Chain, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return importChain(data)
}

func importChain(content []byte) (*Chain, error) {
	var chain *Chain
	if err := json.Unmarshal(content, &chain); err != nil {
		return nil, err
	}
	if engines := chain.Params.Engine; len(engines) != 1 {
		return nil, fmt.Errorf("Expected one consensus engine but found %d", len(engines))
	}
	return chain, nil
}
