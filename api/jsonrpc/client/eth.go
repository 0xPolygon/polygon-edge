package client

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
)

// Eth is used to query the eth service
type Eth struct {
	client *Client
}

// Eth returns a handle on the eth endpoints.
func (c *Client) Eth() *Eth {
	return &Eth{client: c}
}

// BlockNumber returns the number of most recent block.
func (c *Eth) BlockNumber() (uint64, error) {
	var out math.HexOrDecimal64
	err := c.client.do("eth_blockNumber", &out)
	return uint64(out), err
}

// GetBlockByHash returns information about a block by hash.
func (c *Eth) GetBlockByHash(hash common.Hash, full bool) (*types.Block, error) {
	var out *types.Block
	err := c.client.do("eth_getBlockByHash", &out, hash, full)
	return out, err
}

// GetBlockByNumber returns information about a block by block number.
func (c *Eth) GetBlockByNumber(number uint64, full bool) (*types.Block, error) {
	var out *types.Block
	err := c.client.do("eth_getBlockByNumber", &out, math.HexOrDecimal64(number), full)
	return out, err
}
