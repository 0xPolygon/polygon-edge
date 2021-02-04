package minimal

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/0xPolygon/minimal/api"
	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/blockchain/storage/leveldb"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/minimal/keystore"
	"github.com/0xPolygon/minimal/minimal/proto"
	"github.com/0xPolygon/minimal/state"
	"github.com/armon/go-metrics"
	"github.com/armon/go-metrics/prometheus"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/host"
	ma "github.com/multiformats/go-multiaddr"
	"google.golang.org/grpc"

	"github.com/0xPolygon/minimal/protocol"

	libp2pgrpc "github.com/0xPolygon/minimal/helper/grpc"
	itrie "github.com/0xPolygon/minimal/state/immutable-trie"
	"github.com/0xPolygon/minimal/state/runtime/evm"
	"github.com/0xPolygon/minimal/state/runtime/precompiled"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/sealer"
)

/*
var ripemd = types.StringToAddress("0000000000000000000000000000000000000003")

var ripemdFailedTxn = types.StringToHash("0xcf416c536ec1a19ed1fb89e4ec7ffb3cf73aa413b3aa9b77d60e4fd81a4296ba")
*/

// Minimal is the central manager of the blockchain client
type Server struct {
	logger hclog.Logger
	config *Config
	Sealer *sealer.Sealer

	backends  []protocol.Backend
	consensus consensus.Consensus

	// blockchain stack
	blockchain *blockchain.Blockchain
	storage    storage.Storage

	key       *ecdsa.PrivateKey
	chain     *chain.Chain
	apis      []api.API
	InmemSink *metrics.InmemSink
	devMode   bool

	// system grpc server
	grpcServer *grpc.Server

	// libp2p stack
	host         host.Host
	libp2pServer *libp2pgrpc.GRPCProtocol
	addrs        []ma.Multiaddr
}

var dirPaths = []string{
	"blockchain",
	"consensus",
	"keystore",
	"trie",
}

func NewServer(logger hclog.Logger, config *Config) (*Server, error) {
	m := &Server{
		logger:     logger,
		config:     config,
		backends:   []protocol.Backend{},
		apis:       []api.API{},
		chain:      config.Chain,
		grpcServer: grpc.NewServer(),
	}

	m.logger.Info("Data dir", "path", config.DataDir)

	// Generate all the paths in the dataDir
	if err := setupDataDir(config.DataDir, dirPaths); err != nil {
		return nil, fmt.Errorf("failed to create data directories: %v", err)
	}

	/*
		// Only one blockchain backend is allowed
		if len(config.BlockchainEntries) > 1 {
			return nil, fmt.Errorf("Only one blockchain backend allowed")
		}
		// Only one discovery backend is allowed (for now)
		if len(config.DiscoveryEntries) > 1 {
			return nil, fmt.Errorf("Only one discovery mechanism is allowed")
		}
	*/

	/*
		// Build necessary paths
		paths := []string{}
		paths = addPath(paths, "blockchain", nil)
		paths = addPath(paths, "consensus", nil)
		paths = addPath(paths, "network", nil)
		paths = addPath(paths, "trie", nil)

		// Create paths
		if err := setupDataDir(config.DataDir, paths); err != nil {
			return nil, fmt.Errorf("failed to create data directories: %v", err)
		}
	*/

	// Get the private key for the node
	keystore := keystore.NewLocalKeystore(filepath.Join(config.DataDir, "keystore"))
	key, err := keystore.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %v", err)
	}
	m.key = key

	/*
		// setup telemetry
		if err := m.startTelemetry(); err != nil {
			return nil, err
		}
	*/

	storage, err := leveldb.NewLevelDBStorage(filepath.Join(config.DataDir, "blockchain"), logger)
	if err != nil {
		return nil, err
	}
	m.storage = storage

	// Setup consensus
	if err := m.setupConsensus(); err != nil {
		return nil, err
	}

	/*
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
	*/

	/*
		// Start server
		serverConfig := network.DefaultConfig()
		serverConfig.BindAddress = config.BindAddr
		serverConfig.BindPort = config.BindPort
		serverConfig.Bootnodes = config.Chain.Bootnodes
		serverConfig.DataDir = filepath.Join(config.DataDir, "network")

		transport := &rlpx.Rlpx{
			Logger: logger.Named("Rlpx"),
		}

		m.server = network.NewServer("minimal", m.Key, serverConfig, logger.Named("server"), transport)

		// set the peerstore
		// peerstore := network.NewJSONPeerStore(serverConfig.DataDir)
		peerstore, err := network.NewBoltDBPeerStore(serverConfig.DataDir)
		if err != nil {
			return nil, err
		}

		m.server.SetPeerStore(peerstore)
	*/

	/*
		// Build discovery backend
		for name, entry := range config.DiscoveryEntries {
			backend, ok := config.DiscoveryBackends[name]
			if !ok {
				return nil, fmt.Errorf("discovery '%s' not found", name)
			}

			conf := entry.Config
			conf["bootnodes"] = config.Chain.Bootnodes

			// setup discovery factories
			discoveryConfig := &discovery.BackendConfig{
				Logger: logger.Named(name),
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
	*/

	// m.server.SetConsensus(m.consensus)

	stateStorage, err := itrie.NewLevelDBStorage(filepath.Join(m.config.DataDir, "trie"), logger)
	if err != nil {
		return nil, err
	}

	st := itrie.NewState(stateStorage)

	executor := state.NewExecutor(config.Chain.Params, st)
	executor.SetRuntime(precompiled.NewPrecompiled())
	executor.SetRuntime(evm.NewEVM())

	// blockchain object
	m.blockchain = blockchain.NewBlockchain(storage, config.Chain.Params, m.consensus, executor)
	if err := m.blockchain.WriteGenesis(config.Chain.Genesis); err != nil {
		return nil, err
	}

	executor.GetHash = m.blockchain.GetHashHelper

	// Setup sealer
	sealerConfig := &sealer.Config{
		Coinbase: crypto.PubKeyToAddress(&m.key.PublicKey),
	}
	m.Sealer = sealer.NewSealer(sealerConfig, logger, m.blockchain, m.consensus, executor)
	m.Sealer.SetEnabled(m.config.Seal)

	/*
		// Start protocol backends
		for name, entry := range config.ProtocolEntries {
			backend, ok := config.ProtocolBackends[name]
			if !ok {
				return nil, fmt.Errorf("protocol '%s' not found", name)
			}

			proto, err := backend(context.Background(), logger.Named(name), m, entry.Config)
			if err != nil {
				return nil, err
			}
			proto.Run()
			m.backends = append(m.backends, proto)
		}

		// Register backends
		for _, i := range m.backends {
			if err := m.server.RegisterProtocol(i.Protocols()); err != nil {
				return nil, err
			}
		}

		// Start api backends
		for name, entry := range config.APIEntries {
			backend, ok := config.APIBackends[name]
			if !ok {
				return nil, fmt.Errorf("api '%s' not found", name)
			}
			api, err := backend(hcLogger, m, entry.Config)
			if err != nil {
				return nil, err
			}
			m.apis = append(m.apis, api)
		}
	*/

	// setup libp2p server
	if err := m.setupLibP2P(); err != nil {
		return nil, err
	}

	// setup grpc server
	if err := m.setupGRPC(); err != nil {
		return nil, err
	}

	return m, nil
}

func (s *Server) setupConsensus() error {
	engineName := s.config.Chain.Params.GetEngine()
	engine, ok := consensusBackends[engineName]
	if !ok {
		return fmt.Errorf("consensus engine '%s' not found", engineName)
	}

	config := &consensus.Config{
		Params: s.config.Chain.Params,
		Config: s.config.ConsensusConfig,
	}
	config.Config["path"] = filepath.Join(s.config.DataDir, "consensus")

	consensus, err := engine(context.Background(), config, s.key, s.storage, s.logger)
	if err != nil {
		return err
	}
	s.consensus = consensus
	return nil
}

func (s *Server) setupGRPC() error {
	s.grpcServer = grpc.NewServer()

	proto.RegisterSystemServer(s.grpcServer, &systemService{s})

	lis, err := net.Listen("tcp", s.config.GRPCAddr.String())
	if err != nil {
		return err
	}

	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			s.logger.Error(err.Error())
		}
	}()

	s.logger.Info("GRPC server running", "addr", s.config.GRPCAddr.String())
	return nil
}

// Chain returns the chain object of the client
func (s *Server) Chain() *chain.Chain {
	return s.chain
}

func (s *Server) Close() {

	if err := s.blockchain.Close(); err != nil {
		s.logger.Error("failed to close blockchain", "err", err.Error())
	}

	s.host.Close()
	// TODO, close the backends

	/*
		// close the apis
		for _, i := range s.apis {
			i.Close()
		}
	*/
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

func (s *Server) startTelemetry() error {
	s.InmemSink = metrics.NewInmemSink(10*time.Second, time.Minute)
	metrics.DefaultInmemSignal(s.InmemSink)

	metricsConf := metrics.DefaultConfig("minimal")
	metricsConf.EnableHostnameLabel = false
	metricsConf.HostName = ""

	var sinks metrics.FanoutSink

	prom, err := prometheus.NewPrometheusSink()
	if err != nil {
		return err
	}

	sinks = append(sinks, prom)
	sinks = append(sinks, s.InmemSink)

	metrics.NewGlobal(metricsConf, sinks)
	return nil
}
