package api

import (
	"fmt"

	"github.com/umbracle/minimal/api/jsonrpc"
	"github.com/umbracle/minimal/minimal"
)

// Server exposes the api interfaces
type Server struct {
	minimal *minimal.Minimal
}

func NewServer(minimal *minimal.Minimal) (*Server, error) {
	s := &Server{minimal: minimal}

	go s.start()
	return s, nil
}

func (s *Server) start() {
	ss := jsonrpc.Server{}
	fmt.Println(ss)
}
