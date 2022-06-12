package jsonrpc

import (
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/umbracle/ethgo/abi"
)

var (
	ErrStateNotFound = errors.New("given root and slot not found in storage")
)

type Error interface {
	Error() string
	ErrorCode() int
}
type invalidParamsError struct {
	err string
}

type internalError struct {
	err string
}

func (e *internalError) Error() string {
	return e.err
}

func (e *internalError) ErrorCode() int {
	return -32603
}

func (e *invalidParamsError) Error() string {
	return e.err
}

func (e *invalidParamsError) ErrorCode() int {
	return -32602
}

type invalidRequestError struct {
	err string
}

func (e *invalidRequestError) Error() string {
	return e.err
}

func (e *invalidRequestError) ErrorCode() int {
	return -32600
}

type subscriptionNotFoundError struct {
	err string
}

func (e *subscriptionNotFoundError) Error() string {
	return e.err
}

func (e *subscriptionNotFoundError) ErrorCode() int {
	return -32601
}

type methodNotFoundError struct {
	err string
}

func (e *methodNotFoundError) Error() string {
	return e.err
}

func (e *methodNotFoundError) ErrorCode() int {
	return -32601
}

func NewMethodNotFoundError(method string) *methodNotFoundError {
	return &methodNotFoundError{fmt.Sprintf("the method %s does not exist/is not available", method)}
}
func NewInvalidRequestError(msg string) *invalidRequestError {
	return &invalidRequestError{msg}
}
func NewInvalidParamsError(msg string) *invalidParamsError {
	return &invalidParamsError{msg}
}

func NewInternalError(msg string) *internalError {
	return &internalError{msg}
}

func NewSubscriptionNotFoundError(method string) *subscriptionNotFoundError {
	return &subscriptionNotFoundError{fmt.Sprintf("subscribe method %s not found", method)}
}

func constructErrorFromRevert(result *runtime.ExecutionResult) error {
	revertErrMsg, unpackErr := abi.UnpackRevertError(result.ReturnValue)
	if unpackErr != nil {
		return result.Err
	}

	return fmt.Errorf("%w: %s", result.Err, revertErrMsg)
}
