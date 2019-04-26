package agent

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/umbracle/minimal/blockchain/storage"
	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/minimal/keystore"
	"github.com/umbracle/minimal/network/discovery"

	"github.com/umbracle/minimal/protocol"

	metrics "github.com/armon/go-metrics"
	"github.com/armon/go-metrics/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/minimal"

	consensusClique "github.com/umbracle/minimal/consensus/clique"
	consensusEthash "github.com/umbracle/minimal/consensus/ethash"
	consensusPOW "github.com/umbracle/minimal/consensus/pow"

	discoveryConsul "github.com/umbracle/minimal/network/discovery/consul"
	discoveryDevP2P "github.com/umbracle/minimal/network/discovery/devp2p"

	protocolEthereum "github.com/umbracle/minimal/protocol/ethereum"

	storageLevelDB "github.com/umbracle/minimal/blockchain/storage/leveldb"
)

var blockchainBackends = map[string]storage.Factory{
	"leveldb": storageLevelDB.Factory,
}

var consensusBackends = map[string]consensus.Factory{
	"clique": consensusClique.Factory,
	"ethash": consensusEthash.Factory,
	"pow":    consensusPOW.Factory,
}

var discoveryBackends = map[string]discovery.Factory{
	"consul": discoveryConsul.Factory,
	"devp2p": discoveryDevP2P.Factory,
}

var protocolBackends = map[string]protocol.Factory{
	"ethereum": protocolEthereum.Factory,
}

// Agent is a long running daemon that is used to run
// the ethereum client
type Agent struct {
	logger  *log.Logger
	config  *Config
	minimal *minimal.Minimal
}

func NewAgent(logger *log.Logger, config *Config) *Agent {
	return &Agent{logger: logger, config: config}
}

// Start starts the agent
func (a *Agent) Start() error {
	a.startTelemetry()

	var f func(str string) (*chain.Chain, error)
	if _, err := os.Stat(a.config.Chain); err == nil {
		f = chain.ImportFromFile
	} else if os.IsNotExist(err) {
		f = chain.ImportFromName
	} else {
		return fmt.Errorf("Failed to stat (%s): %v", a.config.Chain, err)
	}

	chain, err := f(a.config.Chain)
	if err != nil {
		return fmt.Errorf("failed to load chain %s: %v", a.config.Chain, err)
	}

	// protocol backends
	protocolEntries := map[string]*minimal.Entry{}
	for name, conf := range a.config.Protocols {
		protocolEntries[name] = &minimal.Entry{
			Config: conf,
		}
	}

	// Add by default the ethereum backend if not found
	if _, ok := protocolEntries["ethereum"]; !ok {
		protocolEntries["ethereum"] = &minimal.Entry{
			Config: map[string]interface{}{},
		}
	}

	discoveryEntries := map[string]*minimal.Entry{}
	for name, conf := range a.config.Discovery {
		discoveryEntries[name] = &minimal.Entry{
			Config: conf,
		}
	}

	// If no discovery is specified, set devp2p as default
	if len(discoveryEntries) == 0 {
		discoveryEntries["devp2p"] = &minimal.Entry{
			Config: map[string]interface{}{},
		}
	}

	// blockchain backend
	if a.config.Blockchain == nil {
		return fmt.Errorf("blockchain config not found")
	}
	blockchainEntry := map[string]*minimal.Entry{
		a.config.Blockchain.Backend: &minimal.Entry{
			Config: a.config.Blockchain.Config,
		},
	}

	// consensus backend
	consensusEntry := &minimal.Entry{
		Config: a.config.Consensus,
	}

	config := &minimal.Config{
		Keystore:    keystore.NewLocalKeystore(a.config.DataDir),
		Chain:       chain,
		DataDir:     a.config.DataDir,
		BindAddr:    a.config.BindAddr,
		BindPort:    a.config.BindPort,
		ServiceName: a.config.ServiceName,
		Seal:        a.config.Seal,

		ProtocolBackends: protocolBackends,
		ProtocolEntries:  protocolEntries,

		DiscoveryBackends: discoveryBackends,
		DiscoveryEntries:  discoveryEntries,

		BlockchainBackends: blockchainBackends,
		BlockchainEntries:  blockchainEntry,

		ConsensusBackends: consensusBackends,
		ConsensusEntry:    consensusEntry,
	}

	logger := log.New(os.Stderr, "", log.LstdFlags)
	m, err := minimal.NewMinimal(logger, config)
	if err != nil {
		panic(err)
	}

	a.minimal = m
	return nil
}

func (a *Agent) Close() {
	a.minimal.Close()
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

	l, err := net.Listen("tcp", "localhost:"+strconv.Itoa(a.config.Telemetry.PrometheusPort))
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
