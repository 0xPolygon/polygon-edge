package minimal

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/umbracle/minimal/blockchain/storage/leveldb"
	"github.com/umbracle/minimal/protocol"
	"github.com/umbracle/minimal/state/trie"

	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/network"
	"github.com/umbracle/minimal/network/discovery"
	"github.com/umbracle/minimal/sealer"

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
	Sealer     *sealer.Sealer
	server     *network.Server
	backends   []protocol.Backend
	consensus  consensus.Consensus
	Blockchain *blockchain.Blockchain
	closeCh    chan struct{}
	Key        *ecdsa.PrivateKey
}

func NewMinimal(logger *log.Logger, config *Config) (*Minimal, error) {
	m := &Minimal{
		logger:    logger,
		config:    config,
		sealingCh: make(chan bool, 1),
		closeCh:   make(chan struct{}),
		backends:  []protocol.Backend{},
	}

	key, ok, err := config.Keystore.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %v", err)
	}
	if !ok {
		return nil, fmt.Errorf("private key does not exists")
	}

	m.Key = key

	// Start server
	serverConfig := network.DefaultConfig()
	serverConfig.BindAddress = config.BindAddr
	serverConfig.BindPort = config.BindPort
	serverConfig.Bootnodes = config.Chain.Bootnodes
	serverConfig.DiscoveryBackends = discoveryBackends
	serverConfig.ServiceName = config.ServiceName

	m.server = network.NewServer("minimal", m.Key, serverConfig, logger)

	consensusConfig := &consensus.Config{
		Params: config.Chain.Params,
	}

	engineName := config.Chain.Params.GetEngine()
	engine, ok := consensusBackends[engineName]
	if !ok {
		return nil, fmt.Errorf("consensus engine '%s' not found", engineName)
	}

	m.consensus, err = engine(context.Background(), consensusConfig)
	if err != nil {
		return nil, err
	}

	// blockchain storage
	storage, err := leveldb.NewLevelDBStorage(filepath.Join(m.config.DataDir, "blockchain"), nil)
	if err != nil {
		return nil, err
	}

	trieDB, err := trie.NewLevelDBStorage(filepath.Join(m.config.DataDir, "trie"), logger)
	if err != nil {
		return nil, err
	}

	// blockchain object
	m.Blockchain = blockchain.NewBlockchain(storage, trieDB, m.consensus, config.Chain.Params)
	if err := m.Blockchain.WriteGenesis(config.Chain.Genesis); err != nil {
		return nil, err
	}

	sealerConfig := &sealer.Config{
		CommitInterval: 5 * time.Second,
	}
	m.Sealer = sealer.NewSealer(sealerConfig, logger, m.Blockchain, m.consensus)
	m.Sealer.SetEnabled(m.config.Seal)
	m.Sealer.SetCoinbase(pubkeyToAddress(m.Key.PublicKey))

	// Start backend protocols
	for _, b := range config.ProtocolBackends {
		backend, err := b(context.Background(), m)
		if err != nil {
			return nil, err
		}
		m.backends = append(m.backends, backend)
	}

	// Register backends
	for _, i := range m.backends {
		if err := m.server.RegisterProtocol(i); err != nil {
			return nil, err
		}
	}

	m.server.Schedule()
	return m, nil
}

func (m *Minimal) Close() {
	close(m.closeCh)
	// TODO, add other close methods
}

func pubkeyToAddress(p ecdsa.PublicKey) common.Address {
	pubBytes := crypto.FromECDSAPub(&p)
	return common.BytesToAddress(crypto.Keccak256(pubBytes[1:])[12:])
}
