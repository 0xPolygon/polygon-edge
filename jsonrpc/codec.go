package jsonrpc

import (
	"encoding/json"
	"fmt"
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
