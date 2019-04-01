package jsonrpc

import "encoding/json"

type request struct {
	ID     *json.RawMessage `json:"id"`
	Method string           `json:"method"`
	Params *json.RawMessage `json:"params"`
}

type response struct {
	ID     *json.RawMessage `json:"id"`
	Result interface{}      `json:"result"`
	Error  *errorObject     `json:"error"`
}

type errorObject struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}
