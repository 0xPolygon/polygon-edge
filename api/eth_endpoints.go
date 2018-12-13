package api

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

const (
	PendingBlockNumber  = BlockNumber(-3)
	LatestBlockNumber   = BlockNumber(-2)
	EarliestBlockNumber = BlockNumber(-1)
)

type BlockNumber int64

type Eth struct {
	s *Server
}

func (e *Eth) GetBlockByNumber(in []interface{}, out *interface{}) error {
	var ok bool

	full := false
	if len(in) > 2 {
		return fmt.Errorf("wrong number of params: expected 3 but found %d", len(in))
	}
	if len(in) > 1 {
		if full, ok = in[1].(bool); !ok {
			return convertErr("bool")
		}
	}

	number, ok := in[0].(string)
	if !ok {
		return convertErr("string")
	}
	blocknumber, err := stringToBlockNumber(number)
	if err != nil {
		return err
	}

	if blocknumber < 0 {
		return fmt.Errorf("cannot provide yet this data")
	}

	block := e.s.blockchain.GetBlockByNumber(big.NewInt(int64(blocknumber)), full)
	if block != nil {
		*out = *block
	} else {
		out = nil
	}
	return nil
}

func (e *Eth) GetBlockByHash(in []interface{}, out *interface{}) error {
	var ok bool

	full := false
	if len(in) > 2 {
		return fmt.Errorf("wrong number of params: expected 3 but found %d", len(in))
	}
	if len(in) > 1 {
		if full, ok = in[1].(bool); !ok {
			return convertErr("bool")
		}
	}

	hashStr, ok := in[0].(string)
	if !ok {
		return convertErr("string")
	}
	if strings.HasPrefix(hashStr, "0x") {
		return fmt.Errorf("input is not a hash %s", hashStr)
	}
	hash := common.HexToHash(hashStr)

	block := e.s.blockchain.GetBlockByHash(hash, full)
	if block != nil {
		*out = *block
	} else {
		out = nil
	}
	return nil
}

func (e *Eth) BlockNumber(in interface{}, out *string) error {
	header := e.s.blockchain.Header()
	if header == nil {
		*out = ""
	} else {
		*out = hexutil.Uint64(header.Number.Uint64()).String()
	}
	return nil
}

func convertErr(expected string) error {
	return fmt.Errorf("conversion error: cannot convert to %s", expected)
}

func stringToBlockNumber(str string) (BlockNumber, error) {
	if str == "" {
		return 0, fmt.Errorf("value is empty")
	}

	if strings.HasPrefix(str, "\"") && strings.HasSuffix(str, "\"") {
		switch str := str[1 : len(str)-1]; str {
		case "pending":
			return PendingBlockNumber, nil
		case "latest":
			return LatestBlockNumber, nil
		case "earliest":
			return EarliestBlockNumber, nil
		default:
			return 0, fmt.Errorf("blocknumber not found: %s", str)
		}
	}

	n, err := hexutil.DecodeUint64(str)
	if err != nil {
		return 0, err
	}
	return BlockNumber(n), nil
}
