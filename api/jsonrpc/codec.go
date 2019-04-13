package jsonrpc

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

// Request is a jsonrpc request
type Request struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// Response is a jsonrpc response
type Response struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *ErrorObject    `json:"error,omitempty"`
}

// ErrorObject is a jsonrpc error
type ErrorObject struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Error implements error interface
func (e *ErrorObject) Error() string {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Sprintf("jsonrpc.internal marshal error: %v", err)
	}
	return string(data)
}

const (
	PendingBlockNumber  = BlockNumber(-3)
	LatestBlockNumber   = BlockNumber(-2)
	EarliestBlockNumber = BlockNumber(-1)
)

type BlockNumber int64

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
