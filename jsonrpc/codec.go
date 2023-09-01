package jsonrpc

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

// Request is a jsonrpc request
type Request struct {
	ID     interface{}     `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type BatchRequest []Request

// Response is a jsonrpc response interface
type Response interface {
	GetID() interface{}
	Data() json.RawMessage
	Bytes() ([]byte, error)
}

// ErrorResponse is a jsonrpc error response
type ErrorResponse struct {
	JSONRPC string       `json:"jsonrpc"`
	ID      interface{}  `json:"id,omitempty"`
	Error   *ObjectError `json:"error"`
}

// GetID returns error response id
func (e *ErrorResponse) GetID() interface{} {
	return e.ID
}

// Data returns ObjectError
func (e *ErrorResponse) Data() json.RawMessage {
	data, err := json.Marshal(e.Error)
	if err != nil {
		return json.RawMessage(err.Error())
	}

	return data
}

// Bytes return the serialized response
func (e *ErrorResponse) Bytes() ([]byte, error) {
	return json.Marshal(e)
}

// SuccessResponse is a jsonrpc  success response
type SuccessResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *ObjectError    `json:"error,omitempty"`
}

// GetID returns success response id
func (s *SuccessResponse) GetID() interface{} {
	return s.ID
}

// Data returns the result
func (s *SuccessResponse) Data() json.RawMessage {
	if s.Result != nil {
		return s.Result
	}

	return json.RawMessage("No Data")
}

// Bytes return the serialized response
func (s *SuccessResponse) Bytes() ([]byte, error) {
	return json.Marshal(s)
}

// ObjectError is a jsonrpc error
type ObjectError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Error implements error interface
func (e *ObjectError) Error() string {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Sprintf("jsonrpc.internal marshal error: %v", err)
	}

	return string(data)
}

func (e *ObjectError) MarshalJSON() ([]byte, error) {
	var ds string

	data, ok := e.Data.([]byte)
	if ok && len(data) > 0 {
		ds = "0x" + string(data)
	}

	return json.Marshal(&struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    string `json:"data,omitempty"`
	}{
		Code:    e.Code,
		Message: e.Message,
		Data:    ds,
	})
}

const (
	pending  = "pending"
	latest   = "latest"
	earliest = "earliest"
)

const (
	PendingBlockNumber  = BlockNumber(-3)
	LatestBlockNumber   = BlockNumber(-2)
	EarliestBlockNumber = BlockNumber(-1)
)

type BlockNumber int64

type BlockNumberOrHash struct {
	BlockNumber *BlockNumber `json:"blockNumber,omitempty"`
	BlockHash   *types.Hash  `json:"blockHash,omitempty"`
}

// UnmarshalJSON will try to extract the filter's data.
// Here are the possible input formats :
//
// 1 - "latest", "pending" or "earliest"	- self-explaining keywords
// 2 - "0x2"								- block number #2 (EIP-1898 backward compatible)
// 3 - {blockNumber:	"0x2"}				- EIP-1898 compliant block number #2
// 4 - {blockHash:		"0xe0e..."}			- EIP-1898 compliant block hash 0xe0e...
func (bnh *BlockNumberOrHash) UnmarshalJSON(data []byte) error {
	type bnhCopy BlockNumberOrHash

	var placeholder bnhCopy

	err := json.Unmarshal(data, &placeholder)
	if err != nil {
		number, err := stringToBlockNumber(string(data))
		if err != nil {
			return err
		}

		placeholder.BlockNumber = &number
	}

	// Try to extract object
	bnh.BlockNumber = placeholder.BlockNumber
	bnh.BlockHash = placeholder.BlockHash

	if bnh.BlockNumber != nil && bnh.BlockHash != nil {
		return fmt.Errorf("cannot use both block number and block hash as filters")
	} else if bnh.BlockNumber == nil && bnh.BlockHash == nil {
		return fmt.Errorf("block number and block hash are empty, please provide one of them")
	}

	return nil
}

// stringToBlockNumberSafe parses string and returns Block Number
// works similar to stringToBlockNumber
// but treats empty string or pending block number as a latest
func stringToBlockNumberSafe(str string) (BlockNumber, error) {
	switch str {
	case "", pending:
		return LatestBlockNumber, nil
	default:
		return stringToBlockNumber(str)
	}
}

// stringToBlockNumber parses string and returns Block Number
// empty string is treated as an error
// pending blocks are allowed
func stringToBlockNumber(str string) (BlockNumber, error) {
	if str == "" {
		return 0, fmt.Errorf("value is empty")
	}

	str = strings.Trim(str, "\"")
	switch str {
	case pending:
		return PendingBlockNumber, nil
	case latest:
		return LatestBlockNumber, nil
	case earliest:
		return EarliestBlockNumber, nil
	}

	n, err := common.ParseUint64orHex(&str)
	if err != nil {
		return 0, err
	}

	return BlockNumber(n), nil
}

func createBlockNumberPointer(str string) (*BlockNumber, error) {
	blockNumber, err := stringToBlockNumber(str)
	if err != nil {
		return nil, err
	}

	return &blockNumber, nil
}

// UnmarshalJSON automatically decodes the user input for the block number, when a JSON RPC method is called
func (b *BlockNumber) UnmarshalJSON(buffer []byte) error {
	num, err := stringToBlockNumber(string(buffer))
	if err != nil {
		return err
	}

	*b = num

	return nil
}

// NewRPCErrorResponse is used to create a custom error response
func NewRPCErrorResponse(id interface{}, errCode int, err string, data []byte, jsonrpcver string) Response {
	errObject := &ObjectError{errCode, err, data}

	response := &ErrorResponse{
		JSONRPC: jsonrpcver,
		ID:      id,
		Error:   errObject,
	}

	return response
}

// NewRPCResponse returns Success/Error response object
func NewRPCResponse(id interface{}, jsonrpcver string, reply []byte, err Error) Response {
	var response Response
	switch err.(type) {
	case nil:
		response = &SuccessResponse{JSONRPC: jsonrpcver, ID: id, Result: reply}
	default:
		response = NewRPCErrorResponse(id, err.ErrorCode(), err.Error(), reply, jsonrpcver)
	}

	return response
}
