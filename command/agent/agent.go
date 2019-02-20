package agent

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	metrics "github.com/armon/go-metrics"
	"github.com/armon/go-metrics/prometheus"
	"github.com/hashicorp/go-discover"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/network/discovery"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/blockchain/storage"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/network"
	"github.com/umbracle/minimal/network/rlpx"
	"github.com/umbracle/minimal/protocol"
	"github.com/umbracle/minimal/protocol/ethereum"
	"github.com/umbracle/minimal/syncer"

	consensusClique "github.com/umbracle/minimal/consensus/clique"
	consensusEthash "github.com/umbracle/minimal/consensus/ethash"

	discoveryConsul "github.com/umbracle/minimal/network/discovery/consul"
	discoveryDevP2P "github.com/umbracle/minimal/network/discovery/devp2p"
)

// Agent is a long running daemon that is used to run
// the ethereum client
type Agent struct {
	logger *log.Logger
	config *Config

	server   *network.Server
	discover *discover.Discover
	syncer   *syncer.Syncer
}

func NewAgent(logger *log.Logger, config *Config) *Agent {
	return &Agent{logger: logger, config: config}
}

// Start starts the agent
func (a *Agent) Start() error {
	a.startTelemetry()

	consensusFactory := map[string]consensus.Factory{
		"ethash": consensusEthash.Factory,
		"clique": consensusClique.Factory,
	}

	chain, err := chain.ImportFromName(a.config.Chain)
	if err != nil {
		return fmt.Errorf("Failed to load chain %s: %v", a.config.Chain, err)
	}

	// Create data-dir if it does not exists
	paths := []string{
		"blockchain",
	}
	if err := setupDataDir(a.config.DataDir, paths); err != nil {
		panic(err)
	}

	// Load private key from memory (TODO, do it from a file)
	key, err := loadKey(a.config.DataDir)
	if err != nil {
		panic(err)
	}

	// Start server
	serverConfig := network.DefaultConfig()
	serverConfig.BindAddress = a.config.BindAddr
	serverConfig.BindPort = a.config.BindPort
	serverConfig.Bootnodes = chain.Bootnodes

	serverConfig.DiscoveryBackends = map[string]discovery.Factory{
		"devp2p": discoveryDevP2P.Factory,
		"consul": discoveryConsul.Factory,
	}

	a.server = network.NewServer("minimal", key, serverConfig, a.logger)

	consensusConfig := &consensus.Config{
		Params: chain.Params,
	}
	consensus, err := consensusFactory[chain.Params.GetEngine()](context.Background(), consensusConfig)
	if err != nil {
		panic(err)
	}

	// blockchain storage
	storage, err := storage.NewLevelDBStorage(filepath.Join(a.config.DataDir, "blockchain"), nil)
	if err != nil {
		panic(err)
	}

	// blockchain object
	blockchain := blockchain.NewBlockchain(storage, consensus, chain.Params)
	if err := blockchain.WriteGenesis(chain.Genesis); err != nil {
		panic(err)
	}

	// Start syncer
	syncerConfig := syncer.DefaultConfig()
	syncerConfig.NumWorkers = 1

	// TODO, get network id from chain object
	a.syncer, err = syncer.NewSyncer(1, blockchain, syncerConfig)
	if err != nil {
		panic(err)
	}

	// register protocols
	callback := func(conn rlpx.Conn, peer *network.Peer) protocol.Handler {
		return ethereum.NewEthereumProtocol(conn, peer, a.syncer.GetStatus, blockchain)
	}
	a.server.RegisterProtocol(protocol.ETH63, callback)

	// Start network server work after all the protocols have been registered
	a.server.Schedule()

	// Start the syncer
	go a.syncer.Run()

	// Pipe new added nodes into syncer
	go func() {
		for {
			select {
			case evnt := <-a.server.EventCh:
				if evnt.Type == network.NodeJoin {
					a.syncer.AddNode(evnt.Peer)
				}
			}
		}
	}()

	return nil
}

// TODO, start the api service and connect the internal api with metrics
func (a *Agent) startTelemetry() {
	memSink := metrics.NewInmemSink(10*time.Second, time.Minute)
	metrics.DefaultInmemSignal(memSink)

	metricsConf := metrics.DefaultConfig("minimal")
	metricsConf.EnableHostnameLabel = false
	metricsConf.HostName = ""

	var sinks metrics.FanoutSink

	prom, err := prometheus.NewPrometheusSink()
	if err != nil {
		panic(err)
	}

	sinks = append(sinks, prom)
	sinks = append(sinks, memSink)

	metrics.NewGlobal(metricsConf, sinks)

	l, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(resp http.ResponseWriter, req *http.Request) {
		handler := promhttp.Handler()
		handler.ServeHTTP(resp, req)
	})

	go http.Serve(l, mux)
}

// Close stops the agent
func (a *Agent) Close() {
	// TODO, close syncer first
	a.server.Close()
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

func loadKey(dataDir string) (*ecdsa.PrivateKey, error) {
	path := filepath.Join(dataDir, "./key")

	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("Failed to stat (%s): %v", path, err)
	}
	if !os.IsNotExist(err) {
		// exists
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, err
		}
		key, err := hex.DecodeString(string(data))
		if err != nil {
			return nil, err
		}
		return crypto.ToECDSA(key)
	}

	// it does not exists
	key, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}
	if err := ioutil.WriteFile(path, []byte(hex.EncodeToString(crypto.FromECDSA(key))), 0600); err != nil {
		return nil, err
	}

	return key, nil
}
