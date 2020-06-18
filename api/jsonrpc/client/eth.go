package client

import (
	"fmt"

	"github.com/0xPolygon/minimal/types"
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
	var out string
	if err := c.client.do("eth_blockNumber", &out); err != nil {
		return 0, err
	}
	return types.ParseUint64orHex(&out)
}

// GetBlockByHash returns information about a block by hash.
func (c *Eth) GetBlockByHash(hash types.Hash, full bool) (*types.Block, error) {
	var out *types.Block
	err := c.client.do("eth_getBlockByHash", &out, hash, full)
	return out, err
}

// GetBlockByNumber returns information about a block by block number.
func (c *Eth) GetBlockByNumber(number uint64, full bool) (*types.Block, error) {
	var out *types.Block
	num := []byte(fmt.Sprintf("%#x", number))
	err := c.client.do("eth_getBlockByNumber", &out, num, full)
	return out, err
}
