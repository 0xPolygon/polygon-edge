package minimal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/helper/keccak"
	"github.com/0xPolygon/minimal/jsonrpc"
	"github.com/0xPolygon/minimal/minimal/proto"
	"github.com/0xPolygon/minimal/network"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/state/runtime/system"
	"github.com/0xPolygon/minimal/txpool"
	"github.com/0xPolygon/minimal/types"

	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"

	itrie "github.com/0xPolygon/minimal/state/immutable-trie"
	"github.com/0xPolygon/minimal/state/runtime/evm"
	"github.com/0xPolygon/minimal/state/runtime/precompiled"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/consensus"
)

// Minimal is the central manager of the blockchain client
type Server struct {
	logger hclog.Logger
	config *Config
	state  state.State

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
}

var dirPaths = []string{
	"blockchain",
	"consensus",
	"keystore",
	"trie",
	"libp2p",
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
	workingDirectory = config.DataDir

	// Generate all the paths in the dataDir
	if err := SetupDataDir(config.DataDir, dirPaths); err != nil {
		return nil, fmt.Errorf("failed to create data directories: %v", err)
	}

	// start libp2p
	{
		netConfig := config.Network
		netConfig.Chain = m.config.Chain
		netConfig.DataDir = filepath.Join(m.config.DataDir, "libp2p")

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

	st := itrie.NewState(stateStorage)
	m.state = st

	m.executor = state.NewExecutor(config.Chain.Params, st)

	// Set the consensus helper hub
	m.executor.SetConsensusHub(consensusHubs[config.Chain.Params.GetEngine()])

	// Add the system runtime
	m.executor.SetRuntime(system.NewSystem())

	// Add the precompiled runtime
	m.executor.SetRuntime(precompiled.NewPrecompiled())

	// Add the EVM runtime
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
		if m.txpool, err = txpool.NewTxPool(logger, m.config.Seal, hub, m.grpcServer, m.network); err != nil {
			return nil, err
		}

		// use the eip155 signer
		signer := crypto.NewEIP155Signer(uint64(m.config.Chain.Params.ChainID))
		m.txpool.AddSigner(signer)
	}

	{
		// Setup consensus
		if err := m.setupConsensus(); err != nil {
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

	// setup grpc server
	if err := m.setupGRPC(); err != nil {
		return nil, err
	}

	// setup jsonrpc
	if err := m.setupJSONRPC(); err != nil {
		return nil, err
	}

	if err := m.consensus.Start(); err != nil {
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

var (
	workingDirectory string
)

// setupConsensus sets up the consensus mechanism
func (s *Server) setupConsensus() error {
	engineName := s.config.Chain.Params.GetEngine()
	engine, ok := consensusBackends[engineName]
	if !ok {
		return fmt.Errorf("consensus engine '%s' not found", engineName)
	}

	engineConfig, ok := s.config.Chain.Params.Engine[engineName].(map[string]interface{})
	if !ok {
		engineConfig = map[string]interface{}{}
	}
	config := &consensus.Config{
		Params: s.config.Chain.Params,
		Config: engineConfig,
		Path:   filepath.Join(s.config.DataDir, "consensus"),
		Hub:    consensusHubs[engineName],
	}
	consensus, err := engine(
		context.Background(),
		s.config.Seal,
		config,
		s.txpool,
		s.network,
		s.blockchain,
		s.executor,
		s.grpcServer,
		s.logger.Named("consensus"),
	)
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
	blockCreator, err := j.GetConsensus().GetBlockCreator(header)
	if err != nil {
		return nil, false, err
	}

	transition, err := j.BeginTxn(header.StateRoot, header, blockCreator)

	if err != nil {
		return nil, false, err
	}

	_, failed, err := transition.Apply(txn)

	if err != nil {
		return nil, false, err
	}

	return transition.ReturnValue(), failed, nil
}

var stakingHubInstance StakingHub

// newStakingHub initializes the stakingHubInstance singleton
func newStakingHub() *StakingHub {
	var once sync.Once
	once.Do(func() {
		stakingHubInstance = StakingHub{
			stakingMap:       make(map[types.Address]*big.Int),
			stakingThreshold: big.NewInt(0),
			closeCh:          make(chan struct{}),
		}

		go stakingHubInstance.saveToDisk()
	})

	return &stakingHubInstance
}

// Staking //

// StakingHub acts as a hub (manager) for staked account balances
type StakingHub struct {
	// Address -> Stake
	stakingMap map[types.Address]*big.Int

	// The lowest staked amount in the validator set
	stakingThreshold *big.Int

	// Write-back period (in s) for backup staking data
	writebackPeriod time.Duration

	// Mutex
	stakingMutex sync.Mutex

	// Close channel
	closeCh chan struct{}
}

func (sh *StakingHub) CloseStakingHub() {
	sh.stakingMutex.Lock()
	defer sh.stakingMutex.Unlock()

	// Alert the closing channel
	sh.closeCh <- struct{}{}
}

// saveToDisk is a helper method for periodically saving the stake data to disk
func (sh *StakingHub) saveToDisk() {
	for {
		select {
		case <-sh.closeCh:
			return
		default:
		}

		// Save the current staking map to disk, in JSON
		mappings := sh.GetStakerMappings()

		reader, err := sh.marshalJSON(mappings)
		if err != nil {
			continue
		}

		// Save the json to workingDirectory/stakingMap.json
		file, err := os.Create(filepath.Join(workingDirectory, "stakingMap.json"))
		if err != nil {
			_ = file.Close()
			continue
		}
		_, _ = io.Copy(file, reader)

		// Sleep for the writeback period
		time.Sleep(sh.writebackPeriod * time.Second)
	}
}

// marshalJSON generates the json object for staker mappings
func (sh *StakingHub) marshalJSON(mappings []StakerMapping) (io.Reader, error) {
	sh.stakingMutex.Lock()
	defer sh.stakingMutex.Unlock()

	b, err := json.MarshalIndent(mappings, "", "\t")
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

// isStaker is a helper method to check whether or not an address has a staked balance
func (sh *StakingHub) isStaker(address types.Address) bool {
	if _, ok := sh.stakingMap[address]; ok {
		return true
	}

	return false
}

// IncreaseStake increases the account's staked balance, or sets it if the account wasn't
// in the stakingMap
func (sh *StakingHub) IncreaseStake(address types.Address, stakeBalance *big.Int) {
	sh.stakingMutex.Lock()
	defer sh.stakingMutex.Unlock()

	if !sh.isStaker(address) {
		sh.stakingMap[address] = stakeBalance
	} else {
		sh.stakingMap[address] = big.NewInt(0).Add(sh.stakingMap[address], stakeBalance)
	}
}

// DecreaseStake decreases the account's staked balance if the account is present
func (sh *StakingHub) DecreaseStake(address types.Address, unstakeBalance *big.Int) {
	sh.stakingMutex.Lock()
	defer sh.stakingMutex.Unlock()

	if sh.isStaker(address) {
		sh.stakingMap[address] = big.NewInt(0).Sub(sh.stakingMap[address], unstakeBalance)
	}
}

// ResetStake resets the account's staked balance
func (sh *StakingHub) ResetStake(address types.Address) {
	sh.stakingMutex.Lock()
	defer sh.stakingMutex.Unlock()

	if sh.isStaker(address) {
		delete(sh.stakingMap, address)
	}
}

// GetStakedBalance returns an accounts staked balance if it is a staker.
// Returns 0 if the address is not a staker
func (sh *StakingHub) GetStakedBalance(address types.Address) *big.Int {
	sh.stakingMutex.Lock()
	defer sh.stakingMutex.Unlock()

	if sh.isStaker(address) {
		return sh.stakingMap[address]
	}

	return big.NewInt(0)
}

// GetStakerAddresses returns a list of all addresses that have a stake > 0
func (sh *StakingHub) GetStakerAddresses() []types.Address {
	sh.stakingMutex.Lock()
	defer sh.stakingMutex.Unlock()

	stakers := make([]types.Address, len(sh.stakingMap))

	var indx = 0
	for address := range sh.stakingMap {
		stakers[indx] = address
		indx++
	}

	return stakers
}

// StakerMapping is a representation of a staked account balance
type StakerMapping struct {
	Address types.Address `json:"address"`
	Stake   *big.Int      `json:"stake"`
}

// GetStakerMappings returns the staking addresses and their staking balances
func (sh *StakingHub) GetStakerMappings() []StakerMapping {
	sh.stakingMutex.Lock()
	defer sh.stakingMutex.Unlock()

	mappings := make([]StakerMapping, len(sh.stakingMap))

	var indx = 0
	for address, stake := range sh.stakingMap {
		mappings[indx] = StakerMapping{
			Address: address,
			Stake:   stake,
		}
		indx++
	}

	return mappings
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
