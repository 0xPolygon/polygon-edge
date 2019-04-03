package agent

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/umbracle/minimal/minimal/keystore"

	"github.com/umbracle/minimal/protocol"
	"github.com/umbracle/minimal/protocol/ethereum"

	metrics "github.com/armon/go-metrics"
	"github.com/armon/go-metrics/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/minimal"
)

var protocolBackends = map[string]protocol.Factory{
	"ethereum": ethereum.Factory,
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

	// Create data-dir if it does not exists
	paths := []string{
		"blockchain",
		"trie",
	}
	if err := setupDataDir(a.config.DataDir, paths); err != nil {
		panic(err)
	}

	config := &minimal.Config{
		Keystore:         keystore.NewLocalKeystore(a.config.DataDir),
		Chain:            chain,
		DataDir:          a.config.DataDir,
		BindAddr:         a.config.BindAddr,
		BindPort:         a.config.BindPort,
		ServiceName:      a.config.ServiceName,
		ProtocolBackends: protocolBackends,
		Seal:             a.config.Seal,
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
