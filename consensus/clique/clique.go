package clique

import (
	"context"
	"crypto/ecdsa"

	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
)

// Clique is a consensus algorithm for the clique protocol
type Clique struct {
}

func Factory(ctx context.Context, config *consensus.Config, privateKey *ecdsa.PrivateKey, db storage.Storage, logger hclog.Logger) (consensus.Consensus, error) {
	c := &Clique{}
	return c, nil
}

// VerifyHeader verifies the header is correct
func (c *Clique) VerifyHeader(chain consensus.ChainReader, header *types.Header, uncle, seal bool) error {
	return nil
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (c *Clique) Prepare(chain consensus.ChainReader, header *types.Header) error {
	return nil
}

// Seal seals the block
func (c *Clique) Seal(chain consensus.ChainReader, block *types.Block, ctx context.Context) (*types.Block, error) {
	return nil, nil
}

// Close closes the connection
func (c *Clique) Close() error {
	return nil
}
