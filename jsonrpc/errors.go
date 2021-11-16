package jsonrpc

import (
	"errors"
	"fmt"
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
	e := &methodNotFoundError{fmt.Sprintf("the method %s does not exist/is not available", method)}
	return e
}
func NewInvalidRequestError(msg string) *invalidRequestError {
	e := &invalidRequestError{msg}
	return e
}
func NewInvalidParamsError(msg string) *invalidParamsError {
	e := &invalidParamsError{msg}
	return e
}

func NewInternalError(msg string) *internalError {
	e := &internalError{msg}
	return e
}

func NewSubscriptionNotFoundError(method string) *subscriptionNotFoundError {
	e := &subscriptionNotFoundError{fmt.Sprintf("subscribe method %s not found", method)}
	return e
}
