package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/consensus/ethash"

	"github.com/ethereum/go-ethereum/core"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/network"
	"github.com/umbracle/minimal/protocol"
	"github.com/umbracle/minimal/protocol/ethereum"
	"github.com/umbracle/minimal/storage"
	"github.com/umbracle/minimal/syncer"
)

// mainnet nodes
var peers = []string{}

var mainnetGenesisHash = common.HexToHash("0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3")

func main() {
	fmt.Println("## Minimal ##")

	// -- chain config

	// -- genesis

	mainnetGenesis := core.DefaultGenesisBlock().ToBlock(nil).Header()
	if mainnetGenesis.Hash() != mainnetGenesisHash {
		panic("mainnet block not correct")
	}

	// -- chain config (block forks)

	chainConfig := consensus.NewChainConfig(1150000, 4370000, 0)

	logger := log.New(os.Stderr, "", log.LstdFlags)

	privateKey := "b4c65ef6b82e96fb5f26dc10a79c929985217c078584721e9157c238d1690b22"

	key, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		panic(err)
	}

	// Start network protocol

	config := network.DefaultConfig()
	config.Bootnodes = readFile("./foundation.txt")

	server, err := network.NewServer("minimal", key, config, logger)
	if err != nil {
		panic(err)
	}

	// blockchain storage
	storage, err := storage.NewStorage("/tmp/minimal-test", nil)
	if err != nil {
		panic(err)
	}

	// consensus
	consensus := ethash.NewEthHash(chainConfig)

	// blockchain object
	blockchain := blockchain.NewBlockchain(storage, consensus)
	if err := blockchain.WriteGenesis(mainnetGenesis); err != nil {
		panic(err)
	}

	cc := syncer.DefaultConfig()
	cc.MaxRequests = 1

	syncer, err := syncer.NewSyncer(1, blockchain, cc)
	if err != nil {
		panic(err)
	}

	// register protocols
	callback := func(conn network.Conn, peer *network.Peer) protocol.Handler {
		return ethereum.NewEthereumProtocol(conn, peer, syncer.GetStatus, blockchain)
	}

	server.RegisterProtocol(protocol.ETH63, callback)

	// syncer
	go syncer.Run()

	// connect to some peers

	for _, i := range config.Bootnodes {
		server.Dial(i)
	}

	go func() {
		for {
			select {
			case evnt := <-server.EventCh:
				if evnt.Type == network.NodeJoin {
					fmt.Printf("Node joined: %s\n", evnt.Peer.ID)
					syncer.AddNode(evnt.Peer)
				}
			}
		}
	}()

	handleSignals(server)
}

func handleSignals(s *network.Server) int {
	signalCh := make(chan os.Signal, 4)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	var sig os.Signal
	select {
	case sig = <-signalCh:
	}

	fmt.Printf("Caught signal: %v\n", sig)
	fmt.Printf("Gracefully shutting down agent...\n")

	gracefulCh := make(chan struct{})
	go func() {
		s.Close()
		close(gracefulCh)
	}()

	select {
	case <-signalCh:
		return 1
	case <-time.After(5 * time.Second):
		return 1
	case <-gracefulCh:
		return 0
	}
}

func readFile(s string) []string {
	data, err := ioutil.ReadFile(s)
	if err != nil {
		panic(err)
	}
	return strings.Split(string(data), "\n")
}
