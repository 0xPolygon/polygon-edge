package jsonrpc

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/0xPolygon/polygon-sdk/types"
)

// Request is a jsonrpc request
type Request struct {
	ID     interface{}     `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is a jsonrpc response interface
type Response interface {
	Id() interface{}
	Data() json.RawMessage
	Bytes() ([]byte, error)
}

// ErrorResponse is a jsonrpc error response
type ErrorResponse struct {
	JSONRPC string       `json:"jsonrpc"`
	ID      interface{}  `json:"id,omitempty"`
	Error   *ErrorObject `json:"error"`
}

// Id returns error response id
func (e *ErrorResponse) Id() interface{} {
	return e.ID
}

// Data returns ErrorObject
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
	Error   *ErrorObject    `json:"error,omitempty"`
}

// Id returns success response id
func (s *SuccessResponse) Id() interface{} {
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
func (e *SuccessResponse) Bytes() ([]byte, error) {

	return json.Marshal(e)
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

type BlockNumberOrHash struct {
	BlockNumber *BlockNumber `json:"blockNumber,omitempty"`
	BlockHash   *types.Hash  `json:"blockHash,omitempty"`
}

// Unmarshal will try to extract the filter's data.
// Here are the possible input formats :
//
// 1 - "latest", "pending" or "earliest"	- self-explaining keywords
// 2 - "0x2"								- block number #2 (EIP-1898 backward compatible)
// 3 - {blockNumber:	"0x2"}				- EIP-1898 compliant block number #2
// 4 - {blockHash:		"0xe0e..."}			- EIP-1898 compliant block hash 0xe0e...
func (bnh *BlockNumberOrHash) Unmarshal(input *interface{}) error {
	var placeholder BlockNumberOrHash

	data, err := json.Marshal(*input)
	if err != nil {
		return fmt.Errorf("failed to serialize input: %v", err)
	}

	err = json.Unmarshal(data, &placeholder)
	if err != nil {
		var keyword string
		err = json.Unmarshal(data, &keyword)
		if err == nil {
			// Try to extract keyword
			switch keyword {
			case "pending":
				n := PendingBlockNumber
				bnh.BlockNumber = &n
				return nil
			case "latest":
				n := LatestBlockNumber
				bnh.BlockNumber = &n
				return nil
			case "earliest":
				n := EarliestBlockNumber
				bnh.BlockNumber = &n
				return nil
			default:
				// Try to extract hex number
				s, ok := (*input).(string)
				if !ok {
					return fmt.Errorf("input cannot be converted to string")
				}
				if len(s) < 3 || !strings.HasPrefix(s, "0x") {
					return fmt.Errorf("invalid hexadecimal number provided for block number")
				}
				number, err := strconv.ParseInt(s[2:], 16, 64)
				if err != nil {
					return fmt.Errorf("failed to convert hex string to int64: %v", err)
				}
				bnh.BlockNumber = (*BlockNumber)(&number)
				return nil
			}
		}
		return fmt.Errorf("invalid block number provided")
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

func stringToBlockNumber(str string) (BlockNumber, error) {
	if str == "" {
		return 0, fmt.Errorf("value is empty")
	}

	str = strings.Trim(str, "\"")
	switch str {
	case "pending":
		return PendingBlockNumber, nil
	case "latest":
		return LatestBlockNumber, nil
	case "earliest":
		return EarliestBlockNumber, nil
	}

	n, err := types.ParseUint64orHex(&str)
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

// NewRpcErrorResponse is used to create a custom error response
func NewRpcErrorResponse(id interface{}, errCode int, err string, jsonrpcver string) Response {
	errObject := &ErrorObject{errCode, err, nil}

	response := &ErrorResponse{
		JSONRPC: jsonrpcver,
		ID:      id,
		Error:   errObject,
	}
	return response
}

// NewRpcResponse returns Success/Error response object
func NewRpcResponse(id interface{}, jsonrpcver string, reply []byte, err Error) Response {

	var response Response
	switch err.(type) {
	case nil:
		response = &SuccessResponse{JSONRPC: jsonrpcver, ID: id, Result: reply}
	default:
		response = NewRpcErrorResponse(id, err.ErrorCode(), err.Error(), jsonrpcver)
	}

	return response
}
