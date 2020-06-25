package agent

import (
	"fmt"
	"log"
	"os"

	"github.com/0xPolygon/minimal/api"
	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/minimal/keystore"
	"github.com/0xPolygon/minimal/network/discovery"
	"github.com/google/gops/agent"
	"github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/minimal/protocol"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/minimal"

	consensusClique "github.com/0xPolygon/minimal/consensus/clique"
	consensusEthash "github.com/0xPolygon/minimal/consensus/ethash"
	consensusIBFT "github.com/0xPolygon/minimal/consensus/ibft/backend"
	consensusPOW "github.com/0xPolygon/minimal/consensus/pow"

	discoveryConsul "github.com/0xPolygon/minimal/network/discovery/consul"
	discoveryDevP2P "github.com/0xPolygon/minimal/network/discovery/devp2p"

	protocolEthereum "github.com/0xPolygon/minimal/protocol/ethereum"

	storageBoltDB "github.com/0xPolygon/minimal/blockchain/storage/boltdb"
	storageLevelDB "github.com/0xPolygon/minimal/blockchain/storage/leveldb"

	apiHTTP "github.com/0xPolygon/minimal/api/http"
	apiJsonRPC "github.com/0xPolygon/minimal/api/jsonrpc"
)

var blockchainBackends = map[string]storage.Factory{
	"leveldb": storageLevelDB.Factory,
	"boltdb":  storageBoltDB.Factory,
}

var consensusBackends = map[string]consensus.Factory{
	"clique": consensusClique.Factory,
	"ethash": consensusEthash.Factory,
	"pow":    consensusPOW.Factory,
	"ibft":   consensusIBFT.Factory,
}

var discoveryBackends = map[string]discovery.Factory{
	"consul": discoveryConsul.Factory,
	"devp2p": discoveryDevP2P.Factory,
}

var protocolBackends = map[string]protocol.Factory{
	"ethereum": protocolEthereum.Factory,
}

var apiBackends = map[string]api.Factory{
	"jsonrpc": apiJsonRPC.Factory,
	"http":    apiHTTP.Factory,
}

// Agent is a long running daemon that is used to run
// the ethereum client
type Agent struct {
	logger  *log.Logger
	config  *Config
	minimal *minimal.Minimal
}

func NewAgent(logger *log.Logger, config *Config) *Agent {
	return &Agent{
		logger: logger, config: config,
	}
}

// Start starts the agent
func (a *Agent) Start() error {
	if err := agent.Listen(agent.Options{}); err != nil {
		log.Fatal(err)
	}

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

	// api backends
	apiEntries := map[string]*minimal.Entry{}
	for name, conf := range a.config.API {
		apiEntries[name] = &minimal.Entry{
			Config: conf,
		}
	}
	/*
		// jsonrpc api set by default, can be disabled explicitely on the configuration
		if _, ok := apiEntries["jsonrpc"]; !ok {
			apiEntries["jsonrpc"] = &minimal.Entry{
				Config: map[string]interface{}{},
			}
		}
		// http set by default
		if _, ok := apiEntries["http"]; !ok {
			apiEntries["http"] = &minimal.Entry{
				Config: map[string]interface{}{},
			}
		}
	*/

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

		APIBackends: apiBackends,
		APIEntries:  apiEntries,

		StateStorage: a.config.StateStorage,
	}

	logger := hclog.New(&hclog.LoggerOptions{
		Name:  "minimal",
		Level: hclog.LevelFromString(a.config.LogLevel),
	})

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
