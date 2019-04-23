package minimal

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/umbracle/minimal/chain"

	"github.com/umbracle/minimal/blockchain/storage"
	"github.com/umbracle/minimal/network/discovery"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/umbracle/minimal/protocol"
	trie "github.com/umbracle/minimal/state/immutable-trie"

	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/network"
	"github.com/umbracle/minimal/sealer"
)

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
	Key        *ecdsa.PrivateKey
	chain      *chain.Chain
}

func NewMinimal(logger *log.Logger, config *Config) (*Minimal, error) {
	m := &Minimal{
		logger:    logger,
		config:    config,
		sealingCh: make(chan bool, 1),
		backends:  []protocol.Backend{},
		chain:     config.Chain,
	}

	// Check if the consensus engine exists
	engineName := config.Chain.Params.GetEngine()
	engine, ok := config.ConsensusBackends[engineName]
	if !ok {
		return nil, fmt.Errorf("consensus engine '%s' not found", engineName)
	}

	// Only one blockchain backend is allowed
	if len(config.BlockchainEntries) > 1 {
		return nil, fmt.Errorf("Only one blockchain backend allowed")
	}
	// Only one discovery backend is allowed (for now)
	if len(config.DiscoveryEntries) > 1 {
		return nil, fmt.Errorf("Only one discovery mechanism is allowed")
	}

	// Build necessary paths
	paths := []string{}
	paths = addPath(paths, "blockchain", nil)
	paths = addPath(paths, "consensus", nil)
	paths = addPath(paths, "network", nil)

	// Create paths
	if err := setupDataDir(config.DataDir, paths); err != nil {
		return nil, fmt.Errorf("failed to create data directories: %v", err)
	}

	key, err := config.Keystore.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %v", err)
	}

	// Build storage backend
	var storage storage.Storage
	for name, entry := range config.BlockchainEntries {
		storageFunc, ok := config.BlockchainBackends[name]
		if !ok {
			return nil, fmt.Errorf("storage '%s' not found", name)
		}

		entry.addPath(filepath.Join(config.DataDir, "blockchain"))
		storage, err = storageFunc(entry.Config, logger)
		if err != nil {
			return nil, err
		}
	}

	m.Key = key

	fmt.Println("-- addr --")
	fmt.Println(config.BindAddr)
	fmt.Println(config.BindPort)

	// Start server
	serverConfig := network.DefaultConfig()
	serverConfig.BindAddress = config.BindAddr
	serverConfig.BindPort = config.BindPort
	serverConfig.Bootnodes = config.Chain.Bootnodes
	serverConfig.DataDir = filepath.Join(config.DataDir, "network")

	m.server = network.NewServer("minimal", m.Key, serverConfig, logger)

	// Build discovery backend
	for name, entry := range config.DiscoveryEntries {
		fmt.Printf("Discovery entry: %s\n", name)

		backend, ok := config.DiscoveryBackends[name]
		if !ok {
			return nil, fmt.Errorf("discovery '%s' not found", name)
		}

		conf := entry.Config
		conf["bootnodes"] = config.Chain.Bootnodes

		// setup discovery factories
		discoveryConfig := &discovery.BackendConfig{
			Logger: logger,
			Key:    key,
			Enode:  m.server.Enode,
			Config: conf,
		}

		discovery, err := backend(context.Background(), discoveryConfig)
		if err != nil {
			return nil, err
		}
		m.server.Discovery = discovery
	}

	// Build consensus
	consensusConfig := &consensus.Config{
		Params: config.Chain.Params,
	}
	if config.ConsensusEntry != nil {
		config.ConsensusEntry.addPath(filepath.Join(m.config.DataDir, "consensus"))
	}
	consensusConfig.Config = config.ConsensusEntry.Config

	m.consensus, err = engine(context.Background(), consensusConfig)
	if err != nil {
		return nil, err
	}

	trieDB, err := trie.NewLevelDBStorage(filepath.Join(m.config.DataDir, "trie"), logger)
	if err != nil {
		return nil, err
	}

	st := trie.NewState(trieDB)

	// blockchain object
	m.Blockchain = blockchain.NewBlockchain(storage, st, m.consensus, config.Chain.Params)
	if err := m.Blockchain.WriteGenesis(config.Chain.Genesis); err != nil {
		return nil, err
	}

	sealerConfig := &sealer.Config{
		CommitInterval: 5 * time.Second,
	}
	m.Sealer = sealer.NewSealer(sealerConfig, logger, m.Blockchain, m.consensus)
	m.Sealer.SetEnabled(m.config.Seal)
	m.Sealer.SetCoinbase(pubkeyToAddress(m.Key.PublicKey))

	// Start protocol backends
	for name, entry := range config.ProtocolEntries {
		backend, ok := config.ProtocolBackends[name]
		if !ok {
			return nil, fmt.Errorf("protocol '%s' not found", name)
		}

		proto, err := backend(context.Background(), m, entry.Config)
		if err != nil {
			return nil, err
		}
		m.backends = append(m.backends, proto)
	}

	// Register backends
	for _, i := range m.backends {
		if err := m.server.RegisterProtocol(i); err != nil {
			return nil, err
		}
	}

	if err := m.server.Schedule(); err != nil {
		return nil, err
	}
	return m, nil
}

// Chain returns the chain object of the client
func (m *Minimal) Chain() *chain.Chain {
	return m.chain
}

func (m *Minimal) Close() {
	m.server.Close()
}

func pubkeyToAddress(p ecdsa.PublicKey) common.Address {
	pubBytes := crypto.FromECDSAPub(&p)
	return common.BytesToAddress(crypto.Keccak256(pubBytes[1:])[12:])
}

// Entry is a backend configuration entry
type Entry struct {
	Enabled bool
	Config  map[string]interface{}
}

func (e *Entry) addPath(path string) {
	if len(e.Config) == 0 {
		e.Config = map[string]interface{}{}
	}
	if _, ok := e.Config["path"]; !ok {
		e.Config["path"] = path
	}
}

func addPath(paths []string, path string, entries map[string]*Entry) []string {
	newpath := paths[0:]
	newpath = append(newpath, path)
	for name := range entries {
		newpath = append(newpath, filepath.Join(path, name))
	}
	return newpath
}

func setupDataDir(dataDir string, paths []string) error {
	if err := createDir(dataDir); err != nil {
		return fmt.Errorf("Failed to create data dir: (%s): %v", dataDir, err)
	}

	for _, path := range paths {
		path := filepath.Join(dataDir, path)
		if err := createDir(path); err != nil {
			return fmt.Errorf("Failed to create path: (%s): %v", path, err)
		}
	}
	return nil
}

func createDir(path string) error {
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, os.ModePerm); err != nil {
			return err
		}
	}
	return nil
}

func getSingleKey(i map[string]*Entry) string {
	for k := range i {
		return k
	}
	panic("internal. key not found")
}
