package pow

import (
	"context"
	"crypto/ecdsa"
	crand "crypto/rand"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"time"

	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
)

var (
	two256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))
)

// Pow is a vanilla proof-of-work engine
type Pow struct {
	min uint64
	max uint64
}

func Factory(ctx context.Context, config *consensus.Config, privateKey *ecdsa.PrivateKey, db storage.Storage, logger hclog.Logger) (consensus.Consensus, error) {
	return &Pow{min: 1000000, max: 1500000}, nil
}

func (p *Pow) VerifyHeader(chain consensus.ChainReader, header *types.Header, uncle, seal bool) error {
	parent, _ := chain.CurrentHeader()
	if header.Timestamp <= parent.Timestamp {
		return fmt.Errorf("timestamp lower or equal than parent")
	}
	if header.Difficulty < p.min {
		return fmt.Errorf("Difficulty not correct. '%d' <! '%d'", header.Difficulty, p.min)
	}
	if header.Difficulty > p.max {
		return fmt.Errorf("Difficulty not correct. '%d' >! '%d'", header.Difficulty, p.min)
	}
	return nil
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (p *Pow) Prepare(chain consensus.ChainReader, header *types.Header) error {
	return nil
}

func (p *Pow) Seal(chain consensus.ChainReader, block *types.Block, ctx context.Context) (*types.Block, error) {
	header := block.Header
	header.Difficulty = randomInt(p.min, p.max)

	seed, err := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return nil, err
	}

	rand := rand.New(rand.NewSource(seed.Int64()))
	nonce := uint64(rand.Int63())

	target := new(big.Int).Div(two256, new(big.Int).SetUint64(header.Difficulty))
	aux := big.NewInt(1)

	for {
		header.SetNonce(nonce)
		header.ComputeHash()

		aux.SetBytes(header.Hash.Bytes())
		if aux.Cmp(target) < 0 {
			break
		}
		nonce++

		if ctx.Err() != nil {
			return nil, nil
		}
	}

	block = &types.Block{
		Header:       header.Copy(),
		Transactions: block.Transactions,
	}
	return block, nil
}

func (p *Pow) Close() error {
	return nil
}

func randomInt(min, max uint64) uint64 {
	rand.Seed(time.Now().UnixNano())
	return min + uint64(rand.Intn(int(max-min)))
}
