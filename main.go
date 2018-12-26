package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/trie"

	_ "net/http/pprof"

	metrics "github.com/armon/go-metrics"
	"github.com/armon/go-metrics/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/consensus/ethash"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/umbracle/minimal/blockchain"
	"github.com/umbracle/minimal/network"
	"github.com/umbracle/minimal/protocol"
	"github.com/umbracle/minimal/protocol/ethereum"
	"github.com/umbracle/minimal/storage"
	"github.com/umbracle/minimal/syncer"

	gops "github.com/google/gops/agent"
)

// prometheus monitoring

func startTelemetry() {
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

// mainnet nodes
var peers = []string{}

var mainnetGenesisHash = common.HexToHash("0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3")

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var memprofile = flag.String("memprofile", "", "write memory profile to this file")

func main() {
	fmt.Println("## Minimal ##")

	// -- telemtry
	startTelemetry()

	// start pproff
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	// start gops
	go func() {
		if err := gops.Listen(gops.Options{}); err != nil {
			log.Fatal(err)
		}
	}()

	// -- chain config

	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	// -- load chain

	name := "foundation"

	chain, err := chain.ImportFromName(name)
	if err != nil {
		panic(fmt.Errorf("Failed to load chain %s: %v", name, err))
	}
	genesis := chain.Genesis.ToBlock().Header()

	fmt.Println("-- params --")
	fmt.Println(chain.Params)

	logger := log.New(os.Stderr, "", log.LstdFlags)

	privateKey := "b4c65ef6b82e96fb5f26dc10a79c929985217c078584721e9157c238d1690b22"

	key, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		panic(err)
	}

	// Start network protocol

	config := network.DefaultConfig()
	config.Bootnodes = chain.Bootnodes

	server, err := network.NewServer("minimal", key, config, logger)
	if err != nil {
		panic(err)
	}

	// blockchain storage
	storage, err := storage.NewLevelDBStorage("/home/thor/Desktop/ethereum/minimal-test", nil)
	if err != nil {
		panic(err)
	}

	// consensus
	consensus := ethash.NewEthHash(chain.Params)

	// blockchain object
	blockchain := blockchain.NewBlockchain(storage, consensus)
	if err := blockchain.WriteGenesis(genesis); err != nil {
		panic(err)
	}

	cc := syncer.DefaultConfig()
	cc.NumWorkers = 4

	syncer, err := syncer.NewSyncer(1, blockchain, cc)
	if err != nil {
		panic(err)
	}

	// register protocols
	callback := func(conn network.Conn, peer *network.Peer) protocol.Handler {
		return ethereum.NewEthereumProtocol(conn, peer, syncer.GetStatus, blockchain)
	}

	server.RegisterProtocol(protocol.ETH63, callback)

	/*
		for _, i := range config.Bootnodes {
			server.Dial(i)
		}
	*/

	go syncer.Run()

	// root := common.HexToHash("0xabd629fff6d46c80035f1d7bd5dd47dac51d79f699492a1400418e0a6e6bfa48")

	/*
		stateDB, err := ethdb.NewLDBDatabase("test-3000000", 1000, 1000)
		if err != nil {
			panic(err)
		}

		//h := blockchain.GetHeaderByNumber(big.NewInt(3000000))
		//root := h.Root

		//fmt.Println("Other root")
		//fmt.Println(h.Root.String())

		//root := common.HexToHash("0x0adc4e8e9e6e2316817366304de11332e32e7d831e02f7edb173cabbf0e957a9")

		ss := state.NewStateSync(root, stateDB)
		if ss.Pending() == 0 {
			panic("Already 0")
		}
	*/

	// check if its a full node

	go func() {
		defer func() {
			fmt.Println("finished")
		}()

		for {
			select {
			case evnt := <-server.EventCh:
				if evnt.Type == network.NodeJoin {
					fmt.Println("@@@ ADD NODE @@@")

					/*
						p := newPeer("", evnt.Peer)

						if res, err := p.conn.RequestNodeDataSync([]common.Hash{root}); err != nil {
							fmt.Println("IT IS NOT AN ARCHIVE NODE")
							fmt.Println(res)
						} else {
							fmt.Println("IT IS AN ARCHIVE NODE!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")

							go syncStateTest(stateDB, ss, p)
							return
						}
					*/

					fmt.Println("@@@ ADD NODE @@@")
					syncer.AddNode(evnt.Peer)

				}
			}
		}

	}()

	handleSignals(server)

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.WriteHeapProfile(f)
		f.Close()
		return
	}
}

func syncStateTest(stateDB ethdb.Database, s *trie.Sync, p *Peer) {
	fmt.Println("- start trie sync -")

	keccak := sha3.NewKeccak256()

	count := 0

	for {
		fmt.Printf("%d Pending: %d\n", count, s.Pending())

		// request missing data
		var tasks []common.Hash
		for _, i := range s.Missing(200) {
			tasks = append(tasks, i)
		}
		res, err := p.conn.RequestNodeDataSync(tasks)
		if err != nil {
			fmt.Printf("ERR 4: %s\n", err)
		}

		count += len(res)

		// range over the result
		for _, j := range res {

			r := trie.SyncResult{Data: j}
			keccak.Reset()
			keccak.Write(j)
			keccak.Sum(r.Hash[:0])

			_, _, err := s.Process([]trie.SyncResult{r})
			if err != nil {
				fmt.Printf("ERR 3: %s\n", err)
			}
		}

		// commit values
		b := stateDB.NewBatch()
		if written, err := s.Commit(b); written == 0 || err != nil {
			fmt.Printf("ERR 1: %s\n", err)
		} else if err := b.Write(); err != nil {
			fmt.Printf("ERR 2: %s\n", err)
		}
	}
}

func handleSignals(s *network.Server) int {
	signalCh := make(chan os.Signal, 4)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	var sig os.Signal
	select {
	case sig = <-signalCh:
	}

	fmt.Printf("Caught signal: %v\n", sig)
	fmt.Printf("Gracefully shutting down agent...\n")

	gracefulCh := make(chan struct{})
	go func() {
		s.Close()
		close(gracefulCh)
	}()

	select {
	case <-signalCh:
		return 1
	case <-time.After(5 * time.Second):
		return 1
	case <-gracefulCh:
		return 0
	}
}

func readFile(s string) []string {
	data, err := ioutil.ReadFile(s)
	if err != nil {
		panic(err)
	}
	return strings.Split(string(data), "\n")
}

type Peer struct {
	pretty  string
	id      string
	conn    *ethereum.Ethereum
	peer    *network.Peer
	active  bool
	failed  int
	pending int
}

func newPeer(id string, peer *network.Peer) *Peer {
	return &Peer{
		id:     id,
		active: true,
		pretty: peer.PrettyString(),
		conn:   peer.GetProtocol("eth", 63).(*ethereum.Ethereum),
		peer:   peer,
	}
}
