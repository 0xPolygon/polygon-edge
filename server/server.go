package server

import (
	"context"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/helper/common"
	"github.com/0xPolygon/polygon-sdk/helper/keccak"
	"github.com/0xPolygon/polygon-sdk/jsonrpc"
	"github.com/0xPolygon/polygon-sdk/network"
	"github.com/0xPolygon/polygon-sdk/secrets"
	"github.com/0xPolygon/polygon-sdk/server/proto"
	"github.com/0xPolygon/polygon-sdk/state"
	"github.com/0xPolygon/polygon-sdk/state/runtime"
	"github.com/0xPolygon/polygon-sdk/txpool"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"

	itrie "github.com/0xPolygon/polygon-sdk/state/immutable-trie"
	"github.com/0xPolygon/polygon-sdk/state/runtime/evm"
	"github.com/0xPolygon/polygon-sdk/state/runtime/precompiled"

	"github.com/0xPolygon/polygon-sdk/blockchain"
	"github.com/0xPolygon/polygon-sdk/consensus"
)

// Minimal is the central manager of the blockchain client
type Server struct {
	logger       hclog.Logger
	config       *Config
	state        state.State
	stateStorage itrie.Storage

	consensus consensus.Consensus

	// blockchain stack
	blockchain *blockchain.Blockchain
	chain      *chain.Chain

	// state executor
	executor *state.Executor

	// jsonrpc stack
	jsonrpcServer *jsonrpc.JSONRPC

	// system grpc server
	grpcServer *grpc.Server

	// libp2p network
	network *network.Server

	// transaction pool
	txpool *txpool.TxPool

	serverMetrics *serverMetrics

	prometheusServer *http.Server
	// secrets manager
	secretsManager secrets.SecretsManager
}

var dirPaths = []string{
	"blockchain",
	"keystore",
	"trie",
}

// NewServer creates a new Minimal server, using the passed in configuration
func NewServer(logger hclog.Logger, config *Config) (*Server, error) {
	m := &Server{
		logger:     logger,
		config:     config,
		chain:      config.Chain,
		grpcServer: grpc.NewServer(),
	}

	m.logger.Info("Data dir", "path", config.DataDir)

	// Generate all the paths in the dataDir
	if err := common.SetupDataDir(config.DataDir, dirPaths); err != nil {
		return nil, fmt.Errorf("failed to create data directories: %v", err)
	}

	if config.Telemetry.PrometheusAddr != nil {
		m.serverMetrics = metricProvider("PSDK", config.Chain.Name, true)
		m.prometheusServer = m.startPrometheusServer(config.Telemetry.PrometheusAddr)
	} else {
		m.serverMetrics = metricProvider("PSDK", config.Chain.Name, false)
	}

	// Set up the secrets manager
	if err := m.setupSecretsManager(); err != nil {
		return nil, fmt.Errorf("failed to set up the secrets manager: %v", err)
	}

	// start libp2p
	{
		netConfig := config.Network
		netConfig.Chain = m.config.Chain
		netConfig.DataDir = filepath.Join(m.config.DataDir, "libp2p")
		netConfig.SecretsManager = m.secretsManager

		network, err := network.NewServer(logger, netConfig)
		if err != nil {
			return nil, err
		}
		m.network = network
	}

	// start blockchain object
	stateStorage, err := itrie.NewLevelDBStorage(filepath.Join(m.config.DataDir, "trie"), logger)
	if err != nil {
		return nil, err
	}
	m.stateStorage = stateStorage

	st := itrie.NewState(stateStorage)
	m.state = st

	m.executor = state.NewExecutor(config.Chain.Params, st, logger)
	m.executor.SetRuntime(precompiled.NewPrecompiled())
	m.executor.SetRuntime(evm.NewEVM())

	// compute the genesis root state
	genesisRoot := m.executor.WriteGenesis(config.Chain.Genesis.Alloc)
	config.Chain.Genesis.StateRoot = genesisRoot

	// blockchain object
	m.blockchain, err = blockchain.NewBlockchain(logger, m.config.DataDir, config.Chain, nil, m.executor)
	if err != nil {
		return nil, err
	}

	m.executor.GetHash = m.blockchain.GetHashHelper

	{
		hub := &txpoolHub{
			state:      m.state,
			Blockchain: m.blockchain,
		}
		// start transaction pool
		m.txpool, err = txpool.NewTxPool(
			logger,
			m.config.Seal,
			m.config.Locals,
			m.config.NoLocals,
			m.config.PriceLimit,
			m.config.MaxSlots,
			m.chain.Params.Forks.At(0),
			hub,
			m.grpcServer,
			m.network,
			m.serverMetrics.txpool,
		)
		if err != nil {
			return nil, err
		}

		// use the eip155 signer
		signer := crypto.NewEIP155Signer(uint64(m.config.Chain.Params.ChainID))
		m.txpool.AddSigner(signer)
	}

	{
		// Setup consensus
		if m.consensus, err = SetupConsensus(config, m.txpool, m.network, m.blockchain, m.executor, m.grpcServer, m.secretsManager, m.logger, m.serverMetrics.consensus); err != nil {
			return nil, err
		}
		m.blockchain.SetConsensus(m.consensus)
	}

	// after consensus is done, we can mine the genesis block in blockchain
	// This is done because consensus might use a custom Hash function so we need
	// to wait for consensus because we do any block hashing like genesis
	if err := m.blockchain.ComputeGenesis(); err != nil {
		return nil, err
	}

	// initialize data in consensus layer
	if err := m.consensus.Initialize(); err != nil {
		return nil, err
	}

	// start consensus
	if err := m.consensus.Start(); err != nil {
		return nil, err
	}

	// setup and start grpc server
	if err := m.setupGRPC(); err != nil {
		return nil, err
	}

	// setup and start jsonrpc server
	if err := m.setupJSONRPC(); err != nil {
		return nil, err
	}

	if err := m.network.Start(); err != nil {
		return nil, err
	}

	return m, nil
}

type txpoolHub struct {
	state state.State
	*blockchain.Blockchain
}

func (t *txpoolHub) GetNonce(root types.Hash, addr types.Address) uint64 {
	snap, err := t.state.NewSnapshotAt(root)
	if err != nil {
		return 0
	}
	result, ok := snap.Get(keccak.Keccak256(nil, addr.Bytes()))
	if !ok {
		return 0
	}
	var account state.Account
	if err := account.UnmarshalRlp(result); err != nil {
		return 0
	}
	return account.Nonce
}

func (t *txpoolHub) GetBalance(root types.Hash, addr types.Address) (*big.Int, error) {
	snap, err := t.state.NewSnapshotAt(root)
	if err != nil {
		return nil, fmt.Errorf("unable to get snapshot for root, %v", err)
	}

	result, ok := snap.Get(keccak.Keccak256(nil, addr.Bytes()))
	if !ok {
		return big.NewInt(0), nil
	}

	var account state.Account
	if err = account.UnmarshalRlp(result); err != nil {
		return nil, fmt.Errorf("unable to unmarshal account from snapshot, %v", err)
	}

	return account.Balance, nil
}

// setupSecretsManager sets up the secrets manager
func (s *Server) setupSecretsManager() error {
	secretsManagerConfig := s.config.SecretsManager
	if secretsManagerConfig == nil {
		// No config provided, use default
		secretsManagerConfig = &secrets.SecretsManagerConfig{
			Type: secrets.Local,
		}
	}

	secretsManagerType := secretsManagerConfig.Type
	secretsManagerParams := &secrets.SecretsManagerParams{
		Logger: s.logger,
	}

	if secretsManagerType == secrets.Local {
		// Only the base directory is required for
		// the local secrets manager
		secretsManagerParams.Extra = map[string]interface{}{
			secrets.Path: s.config.DataDir,
		}
	}

	// Grab the factory method
	secretsManagerFactory, ok := secretsManagerBackends[secretsManagerType]
	if !ok {
		return fmt.Errorf("secrets manager type '%s' not found", secretsManagerType)
	}

	// Instantiate the secrets manager
	secretsManager, factoryErr := secretsManagerFactory(
		secretsManagerConfig,
		secretsManagerParams,
	)

	if factoryErr != nil {
		return fmt.Errorf("unable to instantiate secrets manager, %v", factoryErr)
	}

	s.secretsManager = secretsManager

	return nil
}

// SetupConsensus sets up the consensus mechanism
func SetupConsensus(config *Config,
	txPool *txpool.TxPool,
	network *network.Server,
	bc *blockchain.Blockchain,
	executor *state.Executor,
	grpcSrv *grpc.Server,
	secretsManager secrets.SecretsManager,
	logger hclog.Logger,
	metrics *consensus.Metrics,
) (consensus.Consensus, error) {
	engineName := config.Chain.Params.GetEngine()
	engine, ok := consensusBackends[engineName]
	if !ok {
		return nil, fmt.Errorf("consensus engine '%s' not found", engineName)
	}

	engineConfig, ok := config.Chain.Params.Engine[engineName].(map[string]interface{})
	if !ok {
		engineConfig = map[string]interface{}{}
	}
	consensusConfig := &consensus.Config{
		Params: config.Chain.Params,
		Config: engineConfig,
		Path:   filepath.Join(config.DataDir, "consensus"),
	}
	consensus, err := engine(
		&consensus.ConsensusParams{
			Context:        context.Background(),
			Seal:           config.Seal,
			Config:         consensusConfig,
			Txpool:         txPool,
			Network:        network,
			Blockchain:     bc,
			Executor:       executor,
			Grpc:           grpcSrv,
			Logger:         logger.Named("consensus"),
			Metrics:        metrics,
			SecretsManager: secretsManager,
		},
	)
	if err != nil {
		return nil, err
	}
	return consensus, err
}

type jsonRPCHub struct {
	state state.State

	*blockchain.Blockchain
	*txpool.TxPool
	*state.Executor
}

// HELPER + WRAPPER METHODS //

func (j *jsonRPCHub) getState(root types.Hash, slot []byte) ([]byte, error) {
	// the values in the trie are the hashed objects of the keys
	key := keccak.Keccak256(nil, slot)

	snap, err := j.state.NewSnapshotAt(root)
	if err != nil {
		return nil, err
	}
	result, ok := snap.Get(key)
	if !ok {
		return nil, jsonrpc.ErrStateNotFound
	}
	return result, nil
}

func (j *jsonRPCHub) GetAccount(root types.Hash, addr types.Address) (*state.Account, error) {
	obj, err := j.getState(root, addr.Bytes())
	if err != nil {
		return nil, err
	}
	var account state.Account
	if err := account.UnmarshalRlp(obj); err != nil {
		return nil, err
	}
	return &account, nil
}

// GetForksInTime returns the active forks at the given block height
func (j *jsonRPCHub) GetForksInTime(blockNumber uint64) chain.ForksInTime {
	return j.Executor.GetForksInTime(blockNumber)
}

func (j *jsonRPCHub) GetStorage(root types.Hash, addr types.Address, slot types.Hash) ([]byte, error) {
	account, err := j.GetAccount(root, addr)

	if err != nil {
		return nil, err
	}

	obj, err := j.getState(account.Root, slot.Bytes())

	if err != nil {
		return nil, err
	}

	return obj, nil
}

func (j *jsonRPCHub) GetCode(hash types.Hash) ([]byte, error) {
	res, ok := j.state.GetCode(hash)

	if !ok {
		return nil, fmt.Errorf("unable to fetch code")
	}

	return res, nil
}

func (j *jsonRPCHub) ApplyTxn(header *types.Header, txn *types.Transaction) (result *runtime.ExecutionResult, err error) {
	blockCreator, err := j.GetConsensus().GetBlockCreator(header)
	if err != nil {
		return nil, err
	}

	transition, err := j.BeginTxn(header.StateRoot, header, blockCreator)

	if err != nil {
		return
	}

	result, err = transition.Apply(txn)

	return
}

// SETUP //

// setupJSONRCP sets up the JSONRPC server, using the set configuration
func (s *Server) setupJSONRPC() error {
	hub := &jsonRPCHub{
		state:      s.state,
		Blockchain: s.blockchain,
		TxPool:     s.txpool,
		Executor:   s.executor,
	}

	conf := &jsonrpc.Config{
		Store:   hub,
		Addr:    s.config.JSONRPCAddr,
		ChainID: uint64(s.config.Chain.Params.ChainID),
	}

	srv, err := jsonrpc.NewJSONRPC(s.logger, conf)
	if err != nil {
		return err
	}
	s.jsonrpcServer = srv

	return nil
}

// setupGRPC sets up the grpc server and listens on tcp
func (s *Server) setupGRPC() error {
	proto.RegisterSystemServer(s.grpcServer, &systemService{s: s})

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

func (s *Server) Join(addr0 string, dur time.Duration) error {
	return s.network.JoinAddr(addr0, dur)
}

// Close closes the Minimal server (blockchain, networking, consensus)
func (s *Server) Close() {
	// Close the blockchain layer
	if err := s.blockchain.Close(); err != nil {
		s.logger.Error("failed to close blockchain", "err", err.Error())
	}

	// Close the networking layer
	if err := s.network.Close(); err != nil {
		s.logger.Error("failed to close networking", "err", err.Error())
	}

	// Close the consensus layer
	if err := s.consensus.Close(); err != nil {
		s.logger.Error("failed to close consensus", "err", err.Error())
	}

	// Close the state storage
	if err := s.stateStorage.Close(); err != nil {
		s.logger.Error("failed to close storage for trie", "err", err.Error())
	}

	if s.prometheusServer != nil {
		if err := s.prometheusServer.Shutdown(context.Background()); err != nil {
			s.logger.Error("Prometheus server shutdown error", err)
		}
	}
}

// Entry is a backend configuration entry
type Entry struct {
	Enabled bool
	Config  map[string]interface{}
}

// SetupDataDir sets up the polygon-sdk data directory and sub-folders
func SetupDataDir(dataDir string, paths []string) error {
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

func (s *Server) startPrometheusServer(listenAddr *net.TCPAddr) *http.Server {
	srv := &http.Server{
		Addr: listenAddr.String(),
		Handler: promhttp.InstrumentMetricHandler(
			prometheus.DefaultRegisterer, promhttp.HandlerFor(
				prometheus.DefaultGatherer,
				promhttp.HandlerOpts{},
			),
		),
	}

	go func() {
		s.logger.Info("Prometheus server started", "addr=", listenAddr.String())
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			s.logger.Error("Prometheus HTTP server ListenAndServe", "err", err)
		}
	}()

	return srv
}

// createDir creates a file system directory if it doesn't exist
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
