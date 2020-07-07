package ethash

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/helper/dao"
	"github.com/0xPolygon/minimal/helper/keccak"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
)

var (
	two256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))
)

// Ethash is the ethash consensus algorithm
type Ethash struct {
	config   *chain.Params
	cache    *lru.Cache
	fakePow  bool
	path     string
	daoBlock uint64

	keccak256 *keccak.Keccak

	// tmp is the seal hash tmp variable
	tmp []byte
}

// Factory is the factory method to create an Ethash consensus
func Factory(ctx context.Context, config *consensus.Config, privateKey *ecdsa.PrivateKey, db storage.Storage, logger hclog.Logger) (consensus.Consensus, error) {
	var pathStr string
	path, ok := config.Config["path"]
	if ok {
		pathStr, ok = path.(string)
		if !ok {
			return nil, fmt.Errorf("could not convert path to string")
		}
	}

	cache, _ := lru.New(2)
	e := &Ethash{
		config:    config.Params,
		cache:     cache,
		path:      pathStr,
		daoBlock:  dao.DAOForkBlock,
		keccak256: keccak.NewKeccak256(),
	}
	return e, nil
}

// VerifyHeader verifies the header is correct
func (e *Ethash) VerifyHeader(chain consensus.ChainReader, header *types.Header, uncle, seal bool) error {
	headerNum := header.Number
	parent, _ := chain.CurrentHeader()
	parentNum := parent.Number

	if headerNum != parentNum+1 {
		return fmt.Errorf("header and parent are non sequential")
	}
	if header.Timestamp <= parent.Timestamp {
		return fmt.Errorf("incorrect timestamp")
	}

	if uint64(len(header.ExtraData)) > 32 {
		return fmt.Errorf("extradata is too long")
	}

	if uncle {
		// TODO
	} else {
		if int64(header.Timestamp) > time.Now().Add(15*time.Second).Unix() {
			return fmt.Errorf("future block")
		}
	}

	diff := e.CalcDifficulty(int64(header.Timestamp), parent)
	if diff != header.Difficulty {
		return fmt.Errorf("incorrect difficulty")
	}

	cap := uint64(0x7fffffffffffffff)
	if header.GasLimit > cap {
		return fmt.Errorf("incorrect gas limit")
	}

	if header.GasUsed > header.GasLimit {
		return fmt.Errorf("incorrect gas used")
	}

	gas := int64(parent.GasLimit) - int64(header.GasLimit)
	if gas < 0 {
		gas *= -1
	}

	limit := parent.GasLimit / 1024
	if uint64(gas) >= limit || header.GasLimit < 5000 {
		return fmt.Errorf("incorrect gas limit")
	}

	if !e.fakePow {
		// Verify the seal
		number := header.Number
		cache, err := e.getCache(number)
		if err != nil {
			return err
		}

		nonce := binary.BigEndian.Uint64(header.Nonce[:])
		hash := e.sealHash(header)
		digest, result := cache.hashimoto(hash, nonce)

		if !bytes.Equal(header.MixHash[:], digest) {
			return fmt.Errorf("incorrect digest")
		}

		target := new(big.Int).Div(two256, new(big.Int).SetUint64(header.Difficulty))
		if new(big.Int).SetBytes(result).Cmp(target) > 0 {
			return fmt.Errorf("incorrect pow")
		}
	}

	// Verify dao hard fork
	if e.config.ChainID == 1 && header.Number-e.daoBlock < dao.DAOForkExtraDataRange {
		if !bytes.Equal(header.ExtraData[:], dao.DAOForkExtraData) {
			return fmt.Errorf("dao hard fork extradata is not correct")
		}
	}
	return nil
}

// SetDAOBlock sets the dao block, only to be used during tests
func (e *Ethash) SetDAOBlock(n uint64) {
	e.daoBlock = n
}

func (e *Ethash) getCache(blockNumber uint64) (*Cache, error) {
	epoch := blockNumber / uint64(epochLength)
	cache, ok := e.cache.Get(epoch)
	if ok {
		return cache.(*Cache), nil
	}

	cc := newCache(int(epoch))

	if e.path != "" {
		ok, err := cc.Load(e.path)
		if err != nil {
			return nil, err
		}
		if ok {
			e.cache.Add(epoch, cc)
			return cc, nil
		}
	}

	// Cache not loaded from memory
	cc.Build()
	e.cache.Add(epoch, cc)

	if e.path != "" {
		if err := cc.Save(e.path); err != nil {
			return nil, err
		}
	}
	return cc, nil
}

// SetFakePow sets the fakePow flag to true, only used on tests.
func (e *Ethash) SetFakePow() {
	e.fakePow = true
}

// CalcDifficulty calculates the difficulty at a given time.
func (e *Ethash) CalcDifficulty(time int64, parent *types.Header) uint64 {
	next := parent.Number + 1

	switch {
	case e.config.Forks.IsConstantinople(next):
		return MetropolisDifficulty(time, parent, ConstantinopleBombDelay)

	case e.config.Forks.IsByzantium(next):
		return MetropolisDifficulty(time, parent, ByzantiumBombDelay)

	case e.config.Forks.IsHomestead(next):
		return HomesteadDifficulty(time, parent)

	default:
		return FrontierDifficulty(time, parent)
	}
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (e *Ethash) Prepare(chain consensus.ChainReader, header *types.Header) error {
	return nil
}

// Seal seals the block
func (e *Ethash) Seal(chain consensus.ChainReader, block *types.Block, ctx context.Context) (*types.Block, error) {
	panic("NOT IMPLEMENTED")
}

// Close closes the connection
func (e *Ethash) Close() error {
	return nil
}
