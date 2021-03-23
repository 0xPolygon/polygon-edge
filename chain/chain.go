package chain

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"

	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-multierror"
)

var (
	// GenesisGasLimit is the default gas limit of the Genesis block.
	GenesisGasLimit uint64 = 4712388
	// GenesisDifficulty is the default difficulty of the Genesis block.
	GenesisDifficulty = big.NewInt(131072)
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
	Config *Params `json:"config"`

	Nonce      [8]byte       `json:"nonce"`
	Timestamp  uint64        `json:"timestamp"`
	ExtraData  []byte        `json:"extraData,omitempty"`
	GasLimit   uint64        `json:"gasLimit"`
	Difficulty uint64        `json:"difficulty"`
	Mixhash    types.Hash    `json:"mixHash"`
	Coinbase   types.Address `json:"coinbase"`
	Alloc      GenesisAlloc  `json:"alloc,omitempty"`

	// Only for testing
	Number     uint64     `json:"number"`
	GasUsed    uint64     `json:"gasUsed"`
	ParentHash types.Hash `json:"parentHash"`
}

func (g *Genesis) ToBlock() *types.Header {
	head := &types.Header{
		Number:       g.Number,
		Nonce:        g.Nonce,
		Timestamp:    g.Timestamp,
		ParentHash:   g.ParentHash,
		ExtraData:    g.ExtraData,
		GasLimit:     g.GasLimit,
		GasUsed:      g.GasUsed,
		Difficulty:   g.Difficulty,
		MixHash:      g.Mixhash,
		Miner:        g.Coinbase,
		Sha3Uncles:   types.EmptyUncleHash,
		ReceiptsRoot: types.EmptyRootHash,
		TxRoot:       types.EmptyRootHash,
	}
	if g.GasLimit == 0 {
		head.GasLimit = GenesisGasLimit
	}
	if g.Difficulty == 0 {
		head.Difficulty = GenesisDifficulty.Uint64()
	}
	return head
}

// Decoding

func encodeUint64(i uint64) *string {
	if i == 0 {
		return nil
	}

	bs := make([]byte, 8)
	binary.BigEndian.PutUint64(bs, i)

	res := hex.EncodeToHex(bs)
	return &res
}

func encodeBytes(b []byte) *string {
	if len(b) == 0 {
		return nil
	}

	res := hex.EncodeToHex(b[:])
	return &res
}

// MarshalJSON implements the json interface
func (g *Genesis) MarshalJSON() ([]byte, error) {
	type Genesis struct {
		Nonce      string                     `json:"nonce"`
		Timestamp  *string                    `json:"timestamp,omitempty"`
		ExtraData  *string                    `json:"extraData,omitempty"`
		GasLimit   *string                    `json:"gasLimit,omitempty"`
		Difficulty *string                    `json:"difficulty,omitempty"`
		Mixhash    types.Hash                 `json:"mixHash"`
		Coinbase   types.Address              `json:"coinbase"`
		Alloc      *map[string]GenesisAccount `json:"alloc,omitempty"`
		Number     *string                    `json:"number,omitempty"`
		GasUsed    *string                    `json:"gasUsed,omitempty"`
		ParentHash types.Hash                 `json:"parentHash"`
	}

	var enc Genesis
	enc.Nonce = hex.EncodeToHex(g.Nonce[:])

	enc.Timestamp = encodeUint64(g.Timestamp)
	enc.ExtraData = encodeBytes(g.ExtraData)

	enc.GasLimit = encodeUint64(g.GasLimit)
	enc.Difficulty = encodeUint64(g.Difficulty)

	enc.Mixhash = g.Mixhash
	enc.Coinbase = g.Coinbase

	if g.Alloc != nil {
		alloc := make(map[string]GenesisAccount, len(g.Alloc))
		for k, v := range g.Alloc {
			alloc[hex.EncodeToString(k[:])] = v
		}
		enc.Alloc = &alloc
	}

	enc.Number = encodeUint64(g.Number)
	enc.GasUsed = encodeUint64(g.GasUsed)
	enc.ParentHash = g.ParentHash

	return json.Marshal(&enc)
}

// UnmarshalJSON implements the json interface
func (g *Genesis) UnmarshalJSON(data []byte) error {
	type Genesis struct {
		Nonce      *string                   `json:"nonce"`
		Timestamp  *string                   `json:"timestamp"`
		ExtraData  *string                   `json:"extraData"`
		GasLimit   *string                   `json:"gasLimit"`
		Difficulty *string                   `json:"difficulty"`
		Mixhash    *types.Hash               `json:"mixHash"`
		Coinbase   *types.Address            `json:"coinbase"`
		Alloc      map[string]GenesisAccount `json:"alloc"`
		Number     *string                   `json:"number"`
		GasUsed    *string                   `json:"gasUsed"`
		ParentHash *types.Hash               `json:"parentHash"`
	}

	var dec Genesis
	if err := json.Unmarshal(data, &dec); err != nil {
		return err
	}

	var err, subErr error
	parseError := func(field string, subErr error) {
		err = multierror.Append(err, fmt.Errorf("%s: %v", field, subErr))
	}

	nonce, subErr := types.ParseUint64orHex(dec.Nonce)
	if subErr != nil {
		parseError("nonce", subErr)
	}
	binary.BigEndian.PutUint64(g.Nonce[:], nonce)

	g.Timestamp, subErr = types.ParseUint64orHex(dec.Timestamp)
	if subErr != nil {
		parseError("timestamp", subErr)
	}

	if dec.ExtraData != nil {
		g.ExtraData, subErr = types.ParseBytes(dec.ExtraData)
		if subErr != nil {
			parseError("extradata", subErr)
		}
	}

	if dec.GasLimit == nil {
		return fmt.Errorf("field 'gaslimit' is required")
	}
	g.GasLimit, subErr = types.ParseUint64orHex(dec.GasLimit)
	if subErr != nil {
		parseError("gaslimit", subErr)
	}
	g.Difficulty, subErr = types.ParseUint64orHex(dec.Difficulty)
	if subErr != nil {
		parseError("difficulty", subErr)
	}

	if dec.Mixhash != nil {
		g.Mixhash = *dec.Mixhash
	}
	if dec.Coinbase != nil {
		g.Coinbase = *dec.Coinbase
	}
	if dec.Alloc != nil {
		g.Alloc = make(GenesisAlloc, len(dec.Alloc))
		for k, v := range dec.Alloc {
			g.Alloc[types.StringToAddress(k)] = v
		}
	}

	g.Number, subErr = types.ParseUint64orHex(dec.Number)
	if subErr != nil {
		parseError("number", subErr)
	}
	g.GasUsed, subErr = types.ParseUint64orHex(dec.GasUsed)
	if subErr != nil {
		parseError("gasused", subErr)
	}

	if dec.ParentHash != nil {
		g.ParentHash = *dec.ParentHash
	}
	return err
}

// Genesis alloc

// GenesisAccount is an account in the state of the genesis block.
type GenesisAccount struct {
	Code       []byte                    `json:"code,omitempty"`
	Storage    map[types.Hash]types.Hash `json:"storage,omitempty"`
	Balance    *big.Int                  `json:"balance,omitempty"`
	Nonce      uint64                    `json:"nonce,omitempty"`
	PrivateKey []byte                    `json:"secretKey,omitempty"` // for tests
}

// GenesisAlloc specifies the initial state that is part of the genesis block.
type GenesisAlloc map[types.Address]GenesisAccount

// Decoding

func (g *GenesisAccount) UnmarshalJSON(data []byte) error {
	type GenesisAccount struct {
		Code       *string                   `json:"code,omitempty"`
		Storage    map[types.Hash]types.Hash `json:"storage,omitempty"`
		Balance    *string                   `json:"balance"`
		Nonce      *string                   `json:"nonce,omitempty"`
		PrivateKey *string                   `json:"secretKey,omitempty"`
	}

	var dec GenesisAccount
	if err := json.Unmarshal(data, &dec); err != nil {
		return err
	}

	var err error
	var subErr error

	parseError := func(field string, subErr error) {
		err = multierror.Append(err, fmt.Errorf("%s: %v", field, subErr))
	}

	if dec.Code != nil {
		g.Code, subErr = types.ParseBytes(dec.Code)
		if subErr != nil {
			parseError("code", subErr)
		}
	}

	if dec.Storage != nil {
		g.Storage = dec.Storage
	}

	g.Balance, subErr = types.ParseUint256orHex(dec.Balance)
	if subErr != nil {
		parseError("balance", subErr)
	}
	g.Nonce, subErr = types.ParseUint64orHex(dec.Nonce)
	if subErr != nil {
		parseError("nonce", subErr)
	}

	if dec.PrivateKey != nil {
		g.PrivateKey, subErr = types.ParseBytes(dec.PrivateKey)
		if subErr != nil {
			parseError("privatekey", subErr)
		}
	}

	return err
}

func Import(chain string) (*Chain, error) {
	c, err := ImportFromName(chain)
	if err == nil {
		return c, nil
	}
	if chain == "" {
		// use the default genesis.json name
		chain = "./genesis.json"
	}
	return ImportFromFile(chain)
}

// ImportFromName imports a chain from the precompiled json chains (i.e. foundation)
func ImportFromName(chain string) (*Chain, error) {
	data, err := Asset("chain/chains/" + chain + ".json")
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
