package jsonrpc

import (
	"encoding/json"
	"fmt"
)

// Server is an Ethereum server that handles jsonrpc request. It may
// either create a websocket, http or ipc server
type Server struct {
}

func (s *Server) handle(reqBody []byte) error {
	var req request
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return &errorObject{Code: -32600, Message: "invalid json request"}
	}
	if err := s.getMethod(req.Method); err != nil {
		return &errorObject{Code: -32601, Message: fmt.Sprintf("The method %s does not exist/is not available", req.Method)}
	}
	return nil
}

func (s *Server) getMethod(method string) error {
	return nil
}
