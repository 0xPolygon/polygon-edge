package api

import (
	"log"
	"net/http"
	"net/rpc"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/umbracle/minimal/api/jsonrpc"
	"github.com/umbracle/minimal/blockchain"
)

// Server exposes the api interfaces
type Server struct {
	blockchain *blockchain.Blockchain
	endpoints  endpoints
}

func NewServer(blockchain *blockchain.Blockchain) error {
	s := &Server{blockchain: blockchain}

	s.endpoints = endpoints{
		Eth: &Eth{s},
	}

	go s.start()
	return nil
}

func (s *Server) start() {
	r := rpc.NewServer()
	r.Register(s.endpoints.Eth)

	// JsonRPC server
	http.Handle("/jrpc", jsonrpc.ServeHttp(r))

	if err := http.ListenAndServe(":8081", http.DefaultServeMux); err != nil {
		log.Fatalln(err)
	}
}

type endpoints struct {
	Eth *Eth
}

type Eth struct {
	s *Server
}

func (e *Eth) GetBlockByNumber(in *[]interface{}, out *types.Header) error {
	return nil
}
