package network

import (
	"io/ioutil"
	"log"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

type dummyHandler struct{}

func (d *dummyHandler) Init() error {
	return nil
}

func (d *dummyHandler) Close() error {
	return nil
}

func TestServers(config *Config) (*Server, *Server) {
	logger := log.New(ioutil.Discard, "", log.LstdFlags)

	prv0, _ := crypto.GenerateKey()
	prv1, _ := crypto.GenerateKey()

	rand.Seed(int64(time.Now().Nanosecond()))
	port := rand.Intn(9000-5000) + 5000 // Random port between 5000 and 9000

	c0 := *config
	c0.BindPort = port
	s0 := NewServer("test", prv0, &c0, logger)

	c1 := *config
	c1.BindPort = port + 1
	s1 := NewServer("test", prv1, &c1, logger)

	return s0, s1
}
