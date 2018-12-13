package chain

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/gobuffalo/packr"
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
	ExtraData  []byte         `json:"extraData"`
	GasLimit   uint64         `json:"gasLimit"`
	Difficulty *big.Int       `json:"difficulty"`
	Mixhash    common.Hash    `json:"mixHash"`
	Coinbase   common.Address `json:"coinbase"`
	Alloc      GenesisAlloc   `json:"alloc"`

	// Only for testing
	Number     uint64      `json:"number"`
	GasUsed    uint64      `json:"gasUsed"`
	ParentHash common.Hash `json:"parentHash"`
}

// Decoding

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
	if dec.Alloc == nil {
		return errors.New("missing required field 'alloc' for Genesis")
	}
	g.Alloc = make(GenesisAlloc, len(dec.Alloc))
	for k, v := range dec.Alloc {
		g.Alloc[common.Address(k)] = v
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
	PrivateKey []byte // for tests
}

// GenesisAlloc specifies the initial state that is part of the genesis block.
type GenesisAlloc map[common.Address]GenesisAccount

// Decoding

func (g *GenesisAccount) UnmarshalJSON(data []byte) error {
	type GenesisAccount struct {
		Code       *hexutil.Bytes              `json:"code,omitempty"`
		Storage    map[common.Hash]common.Hash `json:"storage,omitempty"`
		Balance    *math.HexOrDecimal256       `json:"balance" gencodec:"required"`
		Nonce      *math.HexOrDecimal64        `json:"nonce,omitempty"`
		PrivateKey *hexutil.Bytes              `json:"secretKey,omitempty"`
	}

	var dec GenesisAccount
	if err := json.Unmarshal(data, &dec); err != nil {
		return err
	}

	if dec.Code != nil {
		g.Code = *dec.Code
	}
	g.Storage = dec.Storage
	if dec.Balance == nil {
		return errors.New("missing required field 'balance' for GenesisAccount")
	}
	g.Balance = (*big.Int)(dec.Balance)
	if dec.Nonce != nil {
		g.Nonce = uint64(*dec.Nonce)
	}
	if dec.PrivateKey != nil {
		g.PrivateKey = *dec.PrivateKey
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
	err := json.Unmarshal(content, &chain)
	return chain, err
}
