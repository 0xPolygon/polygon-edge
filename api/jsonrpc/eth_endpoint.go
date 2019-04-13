package jsonrpc

import (
	"fmt"
	"math/big"
)

// Eth is the eth jsonrpc endpoint
type Eth struct {
	s *Server
}

// GetBlockByNumber returns information about a block by block number
func (e *Eth) GetBlockByNumber(blockNumber string, full bool) (interface{}, error) {
	block, err := stringToBlockNumber(blockNumber)
	if err != nil {
		return nil, err
	}
	if block < 0 {
		return nil, fmt.Errorf("this data cannot be provided yet")
	}

	// TODO, show full blocks
	header, _ := e.s.minimal.Blockchain.GetHeaderByNumber(big.NewInt(int64(block)))
	return header, nil
}

// GetBlockByHash returns information about a block by hash
func (e *Eth) GetBlockByHash(hashStr string, full bool) (interface{}, error) {
	return nil, nil
}
