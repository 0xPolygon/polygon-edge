package minimal

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/blockchain/storage/leveldb"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/helper/keccak"
	"github.com/0xPolygon/minimal/jsonrpc"
	"github.com/0xPolygon/minimal/minimal/keystore"
	"github.com/0xPolygon/minimal/minimal/proto"
	"github.com/0xPolygon/minimal/network"
	"github.com/0xPolygon/minimal/protocol"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/txpool"
	"github.com/0xPolygon/minimal/types"

	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"

	itrie "github.com/0xPolygon/minimal/state/immutable-trie"
	"github.com/0xPolygon/minimal/state/runtime/evm"
	"github.com/0xPolygon/minimal/state/runtime/precompiled"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/sealer"
)

// Minimal is the central manager of the blockchain client
type Server struct {
	logger hclog.Logger
	config *Config
	Sealer *sealer.Sealer
	state  state.State

	consensus consensus.Consensus

	// blockchain stack
	blockchain *blockchain.Blockchain
	storage    storage.Storage

	key   *ecdsa.PrivateKey // TODO: Remove
	chain *chain.Chain

	// jsonrpc stack
	jsonrpcServer *jsonrpc.JSONRPC

	// system grpc server
	grpcServer *grpc.Server

	// libp2p network
	network *network.Server

	// transaction pool
	txpool *txpool.TxPool

	// syncer protocol
	syncer *protocol.Syncer
}

var dirPaths = []string{
	"blockchain",
	"consensus",
	"keystore",
	"trie",
	"libp2p",
}

func NewServer(logger hclog.Logger, config *Config) (*Server, error) {
	m := &Server{
		logger:     logger,
		config:     config,
		chain:      config.Chain,
		grpcServer: grpc.NewServer(),
	}

	m.logger.Info("Data dir", "path", config.DataDir)

	// Generate all the paths in the dataDir
	if err := setupDataDir(config.DataDir, dirPaths); err != nil {
		return nil, fmt.Errorf("failed to create data directories: %v", err)
	}

	// Get the private key for the node
	keystore := keystore.NewLocalKeystore(filepath.Join(config.DataDir, "keystore"))
	key, err := keystore.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %v", err)
	}
	m.key = key

	// start libp2p
	{
		netConfig := config.Network
		netConfig.DataDir = filepath.Join(m.config.DataDir, "libp2p")

		m.network, err = network.NewServer(logger, netConfig)
		if err != nil {
			return nil, err
		}
	}

	storage, err := leveldb.NewLevelDBStorage(filepath.Join(config.DataDir, "blockchain"), logger)
	if err != nil {
		return nil, err
	}
	m.storage = storage

	// start blockchain object
	stateStorage, err := itrie.NewLevelDBStorage(filepath.Join(m.config.DataDir, "trie"), logger)
	if err != nil {
		return nil, err
	}

	st := itrie.NewState(stateStorage)
	m.state = st

	executor := state.NewExecutor(config.Chain.Params, st)
	executor.SetRuntime(precompiled.NewPrecompiled())
	executor.SetRuntime(evm.NewEVM())

	// blockchain object
	m.blockchain, err = blockchain.NewBlockchain(logger, storage, config.Chain, m.consensus, executor)
	if err != nil {
		return nil, err
	}

	executor.GetHash = m.blockchain.GetHashHelper

	{
		// start transaction pool
		if m.txpool, err = txpool.NewTxPool(m.blockchain, m.config.Chain, m.network); err != nil {
			return nil, err
		}
	}

	{
		// Setup consensus
		if err := m.setupConsensus(); err != nil {
			return nil, err
		}
		m.blockchain.SetConsensus(m.consensus)
	}

	// Setup sealer
	sealerConfig := &sealer.Config{
		Coinbase: crypto.PubKeyToAddress(&m.key.PublicKey),
	}
	m.Sealer = sealer.NewSealer(sealerConfig, logger, m.blockchain, m.consensus, executor)
	m.Sealer.SetEnabled(m.config.Seal)

	// setup grpc server
	if err := m.setupGRPC(); err != nil {
		return nil, err
	}

	// setup jsonrpc
	if err := m.setupJSONRPC(); err != nil {
		return nil, err
	}

	// setup syncer protocol
	m.syncer = protocol.NewSyncer(logger, m.network, m.blockchain)
	m.syncer.Start()

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

	consensus, err := engine(context.Background(), config, s.txpool, s.blockchain, s.blockchain.Executor(), s.key, s.logger.Named("consensus"))
	if err != nil {
		return err
	}
	s.consensus = consensus
	return nil
}

type jsonRPCHub struct {
	state state.State

	*blockchain.Blockchain
	*txpool.TxPool
	*state.Executor
}

func (j *jsonRPCHub) getState(root types.Hash, slot []byte) ([]byte, error) {
	// the values in the trie are the hashed objects of the keys
	key := keccak.Keccak256(nil, slot)

	snap, err := j.state.NewSnapshotAt(root)
	if err != nil {
		return nil, err
	}
	result, ok := snap.Get(key)
	if !ok {
		return nil, fmt.Errorf("error getting storage snapshot")
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

func (j *jsonRPCHub) ApplyTxn(header *types.Header, txn *types.Transaction) ([]byte, bool, error) {
	transition, err := j.BeginTxn(header.StateRoot, header)
	if err != nil {
		return nil, false, err
	}

	_, failed, err := transition.Apply(txn)
	if err != nil {
		return nil, false, err
	}
	return transition.ReturnValue(), failed, nil
}

func (s *Server) setupJSONRPC() error {
	hub := &jsonRPCHub{
		state:      s.state,
		Blockchain: s.blockchain,
		TxPool:     s.txpool,
		Executor:   s.blockchain.Executor(),
	}

	conf := &jsonrpc.Config{
		Store: hub,
		Addr:  s.config.JSONRPCAddr,
	}

	srv, err := jsonrpc.NewJSONRPC(s.logger, conf)
	if err != nil {
		return err
	}
	s.jsonrpcServer = srv
	return nil
}

func (s *Server) setupGRPC() error {
	s.grpcServer = grpc.NewServer()

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

func (s *Server) Close() {
	if err := s.blockchain.Close(); err != nil {
		s.logger.Error("failed to close blockchain", "err", err.Error())
	}
	s.network.Close()
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
