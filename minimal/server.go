package minimal

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/blockchain/storage/leveldb"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/jsonrpc"
	"github.com/0xPolygon/minimal/minimal/keystore"
	"github.com/0xPolygon/minimal/minimal/proto"
	"github.com/0xPolygon/minimal/network"
	"github.com/0xPolygon/minimal/protocol"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/types"

	"github.com/armon/go-metrics"
	"github.com/armon/go-metrics/prometheus"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/host"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	ma "github.com/multiformats/go-multiaddr"
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

	key       *ecdsa.PrivateKey
	chain     *chain.Chain
	InmemSink *metrics.InmemSink
	devMode   bool

	// jsonrpc stack
	jsonrpcServer *jsonrpc.JSONRPC

	// system grpc server
	grpcServer *grpc.Server

	// libp2p stack
	host host.Host
	// libp2pServer    *libp2pgrpc.GRPCProtocol
	addrs           []ma.Multiaddr
	dht             *dht.IpfsDHT
	peerAddedCh     chan struct{}
	peerRemovedCh   chan struct{}
	peerJoined      map[string]bool
	peerJoinedMutex *sync.RWMutex
	//host         host.Host
	//libp2pServer *libp2pgrpc.GRPCProtocol
	//addrs        []ma.Multiaddr
	network *network.Server

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
		logger: logger,
		config: config,
		// backends:   []protocol.Backend{},
		chain:           config.Chain,
		grpcServer:      grpc.NewServer(),
		peerAddedCh:     make(chan struct{}),
		peerRemovedCh:   make(chan struct{}),
		peerJoined:      make(map[string]bool),
		peerJoinedMutex: new(sync.RWMutex),
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

	storage, err := leveldb.NewLevelDBStorage(filepath.Join(config.DataDir, "blockchain"), logger)
	if err != nil {
		return nil, err
	}
	m.storage = storage

	// Setup consensus
	if err := m.setupConsensus(); err != nil {
		return nil, err
	}

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

	// Setup sealer
	sealerConfig := &sealer.Config{
		Coinbase: crypto.PubKeyToAddress(&m.key.PublicKey),
	}
	m.Sealer = sealer.NewSealer(sealerConfig, logger, m.blockchain, m.consensus, executor)
	m.Sealer.SetEnabled(m.config.Seal)

	// start libp2p
	m.network, err = network.NewServer(logger, filepath.Join(m.config.DataDir, "libp2p"), config.LibP2PAddr)
	if err != nil {
		return nil, err
	}

	/*
		// setup libp2p server
		if err := m.setupLibP2P(); err != nil {
			return nil, err
		}
	*/

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
	// m.syncer.Register(m.libp2pServer.GetGRPCServer())
	m.syncer.Start()

	// register the libp2p GRPC endpoints
	//proto.RegisterHandshakeServer(m.libp2pServer.GetGRPCServer(), &handshakeService{s: m})

	//m.libp2pServer.Serve()
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

type jsonRPCHub struct {
	state state.State

	*blockchain.Blockchain
	*sealer.Sealer
	*state.Executor
}

func (j *jsonRPCHub) getState(root types.Hash, slot []byte) ([]byte, error) {
	snap, err := j.state.NewSnapshotAt(root)
	if err != nil {
		return nil, err
	}
	result, ok := snap.Get(slot)
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
		Sealer:     s.Sealer,
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

func (s *Server) Join(addr0 string) error {
	s.logger.Info("Join peer", "addr", addr0)

	if err := s.network.ConnectAddr(addr0); err != nil {
		panic(err)
		return err
	}
	return nil

	/*
			// add peer to the libp2p peerstore
			peerID, err := s.AddPeerFromMultiAddrString(addr0)
			if err != nil {
				return err
			}

			// perform handshake protocol
			conn, err := s.dial(peerID)
			if err != nil {
				return err
			}
			clt := proto.NewHandshakeClient(conn)

		if s.getPeerJoined(addr0) {
			return nil
		}
		s.setPeerJoined(addr0, true)
		go s.syncer.HandleUser(peerID, conn)
			req := &proto.HelloReq{
				Id: s.host.ID().String(),
			}
			if _, err := clt.Hello(context.Background(), req); err != nil {
				return err
			}

			// send the connection to the syncer
			go s.syncer.HandleUser(peerID, conn)
	*/

	return nil
}

/*
func (s *Server) handleConnUser(addr string) {
	// we are already connected with libp2p
	peerID, err := peer.Decode(addr)
	if err != nil {
		panic(err)
	}

	peerInfo := s.host.Peerstore().PeerInfo(peerID)
	addr0 := AddrInfoToString(&peerInfo)
	if s.getPeerJoined(addr0) {
		return
	}

	// perform handshake protocol
	conn, err := s.dial(peerID)
	if err != nil {
		panic(err)
	}

	s.setPeerJoined(addr0, true)
	go s.syncer.HandleUser(peerID, conn)
}
*/

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

func (s *Server) getPeerJoined(addr0 string) bool {
	s.peerJoinedMutex.RLock()
	defer s.peerJoinedMutex.RUnlock()
	return s.peerJoined[addr0]
}

func (s *Server) setPeerJoined(addr0 string, joined bool) {
	s.peerJoinedMutex.Lock()
	defer s.peerJoinedMutex.Unlock()
	s.peerJoined[addr0] = joined
}
