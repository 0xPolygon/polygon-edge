package jsonrpc

import "errors"

var (
	ErrStateNotFound = errors.New("given root and slot not found in storage")
)
