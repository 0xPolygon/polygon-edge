package minimal

import (
	"context"
	"fmt"
	"log"
	"net"
	"path/filepath"
	"time"

	"github.com/umbracle/minimal/state/trie"

	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/blockchain/storage"
	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/network"
	"github.com/umbracle/minimal/network/discovery"
	"github.com/umbracle/minimal/protocol"
	"github.com/umbracle/minimal/protocol/ethereum"
	"github.com/umbracle/minimal/sealer"
	"github.com/umbracle/minimal/syncer"

	consensusClique "github.com/umbracle/minimal/consensus/clique"
	consensusEthash "github.com/umbracle/minimal/consensus/ethash"
	consensusPOW "github.com/umbracle/minimal/consensus/pow"

	discoveryConsul "github.com/umbracle/minimal/network/discovery/consul"
)

var consensusBackends = map[string]consensus.Factory{
	"clique": consensusClique.Factory,
	"ethash": consensusEthash.Factory,
	"pow":    consensusPOW.Factory,
}

var discoveryBackends = map[string]discovery.Factory{
	"consul": discoveryConsul.Factory,
	// "devp2p": discoveryDevP2P.Factory,
}

// Minimal is the central manager of the blockchain client
type Minimal struct {
	logger     *log.Logger
	config     *Config
	sealingCh  chan bool
	sealer     *sealer.Sealer
	server     *network.Server
	syncer     *syncer.Syncer
	consensus  consensus.Consensus
	blockchain *blockchain.Blockchain
	closeCh    chan struct{}
}

func NewMinimal(logger *log.Logger, config *Config) *Minimal {
	m := &Minimal{
		logger:    logger,
		config:    config,
		sealingCh: make(chan bool, 1),
		closeCh:   make(chan struct{}),
	}

	fmt.Println(config.BindAddr)

	// Start server
	serverConfig := network.DefaultConfig()
	serverConfig.BindAddress = config.BindAddr
	serverConfig.BindPort = config.BindPort
	serverConfig.Bootnodes = config.Chain.Bootnodes
	serverConfig.DiscoveryBackends = discoveryBackends
	serverConfig.ServiceName = config.ServiceName

	m.server = network.NewServer("minimal", config.Key, serverConfig, logger)

	consensusConfig := &consensus.Config{
		Params: config.Chain.Params,
	}

	var err error
	m.consensus, err = consensusBackends[config.Chain.Params.GetEngine()](context.Background(), consensusConfig)
	if err != nil {
		panic(err)
	}

	// blockchain storage
	storage, err := storage.NewLevelDBStorage(filepath.Join(m.config.DataDir, "blockchain"), nil)
	if err != nil {
		panic(err)
	}

	trieDB, err := trie.NewLevelDBStorage(filepath.Join(m.config.DataDir, "trie"), logger)
	if err != nil {
		panic(err)
	}

	// blockchain object
	m.blockchain = blockchain.NewBlockchain(storage, trieDB, m.consensus, config.Chain.Params)
	if err := m.blockchain.WriteGenesis(config.Chain.Genesis); err != nil {
		panic(err)
	}

	// Start syncer
	syncerConfig := syncer.DefaultConfig()
	syncerConfig.NumWorkers = 1

	// TODO, get network id from chain object
	m.syncer, err = syncer.NewSyncer(1, m.blockchain, syncerConfig)
	if err != nil {
		panic(err)
	}

	// register protocols
	callback := func(conn net.Conn, peer *network.Peer) protocol.Handler {
		return ethereum.NewEthereumProtocol(conn, peer, m.syncer.GetStatus, m.blockchain)
	}
	m.server.RegisterProtocol(protocol.ETH63, callback)

	// Start network server work after all the protocols have been registered
	m.server.Schedule()

	// Pipe new added nodes into syncer
	go m.listenServerEvents()

	// Start sealer
	sealerConfig := &sealer.Config{
		CommitInterval: 1 * time.Second, // TODO, where does it comes from this value?
	}
	m.sealer = sealer.NewSealer(sealerConfig, logger, m.blockchain, m.consensus)

	// Enable the sealer by default. If new blocks arrive and he finds out he is lagging behind
	// it will stop the sealing. NOTE: Maybe it would be better to wait for the first peer we connect?
	m.sealer.SetEnabled(true)

	return m
}

func (m *Minimal) Close() {
	close(m.closeCh)
	// TODO, add other close methods
}

func (m *Minimal) listenServerEvents() {
	for {
		select {
		case evnt := <-m.server.EventCh:

			fmt.Println("NEW NODE CONNECTED. DOING NOTHING")
			fmt.Println(evnt.Type)

			/*
				if evnt.Type == network.NodeJoin {
					m.syncer.AddNode(evnt.Peer)
				}
			*/
		case <-m.closeCh:
			return
		}
	}
}
