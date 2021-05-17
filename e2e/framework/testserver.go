package framework

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/compiler"
	"github.com/umbracle/go-web3/jsonrpc"
	"github.com/umbracle/go-web3/testutil"
	"golang.org/x/crypto/sha3"

	"google.golang.org/grpc"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/minimal/proto"
	txpoolProto "github.com/0xPolygon/minimal/txpool/proto"
	"github.com/0xPolygon/minimal/types"
)

type ConsensusType int

const (
	ConsensusIBFT ConsensusType = iota
	ConsensusDummy
)

type SrvAccount struct {
	Addr    types.Address
	Balance *big.Int
}

// TestServerConfig for the test server
type TestServerConfig struct {
	JsonRPCPort  int64         // The JSON RPC endpoint port
	GRPCPort     int64         // The GRPC endpoint port
	LibP2PPort   int64         // The Libp2p endpoint port
	Seal         bool          // Flag indicating if blocks should be sealed
	DataDir      string        // The directory for the data files
	PremineAccts []*SrvAccount // Accounts with existing balances (genesis accounts)
	DevMode      bool          // Toggles the dev mode
	Consensus    ConsensusType // Consensus Type
}

// CALLBACKS //

// Premine callback specifies an account with a balance (in WEI)
func (t *TestServerConfig) Premine(addr types.Address, amount *big.Int) {
	if t.PremineAccts == nil {
		t.PremineAccts = []*SrvAccount{}
	}
	t.PremineAccts = append(t.PremineAccts, &SrvAccount{
		Addr:    addr,
		Balance: amount,
	})
}

// SetDev callback toggles the dev mode
func (t *TestServerConfig) SetDev(state bool) {
	t.DevMode = state
}

// SetDev callback toggles the dev mode
func (t *TestServerConfig) SetConsensus(c ConsensusType) {
	t.Consensus = c
}

// SetSeal callback toggles the seal mode
func (t *TestServerConfig) SetSeal(state bool) {
	t.Seal = state
}

type TestServerConfigCallback func(*TestServerConfig)

var initialPort = int64(12000)

func getOpenPort() int64 {
	port := atomic.AddInt64(&initialPort, 1)
	return port
}

type TestServer struct {
	t *testing.T

	Config *TestServerConfig
	cmd    *exec.Cmd
}

func (t *TestServer) GrpcAddr() string {
	return fmt.Sprintf("http://127.0.0.1:%d", t.Config.GRPCPort)
}

func (t *TestServer) JsonRPCAddr() string {
	return fmt.Sprintf("http://127.0.0.1:%d", t.Config.JsonRPCPort)
}

func (t *TestServer) JSONRPC() *jsonrpc.Client {
	clt, err := jsonrpc.NewClient(t.JsonRPCAddr())
	if err != nil {
		t.t.Fatal(err)
	}
	return clt
}

func (t *TestServer) Operator() proto.SystemClient {
	conn, err := grpc.Dial(fmt.Sprintf("127.0.0.1:%d", t.Config.GRPCPort), grpc.WithInsecure())
	if err != nil {
		t.t.Fatal(err)
	}
	return proto.NewSystemClient(conn)
}

func (t *TestServer) TxnPoolOperator() txpoolProto.TxnPoolOperatorClient {
	conn, err := grpc.Dial(fmt.Sprintf("127.0.0.1:%d", t.Config.GRPCPort), grpc.WithInsecure())
	if err != nil {
		t.t.Fatal(err)
	}
	return txpoolProto.NewTxnPoolOperatorClient(conn)
}

func (t *TestServer) Stop() {
	if err := t.cmd.Process.Kill(); err != nil {
		t.t.Error(err)
	}
}

func NewTestServerFromGenesis(t *testing.T) *TestServer {
	c, err := chain.ImportFromFile("../genesis.json")
	assert.NoError(t, err)

	config := &TestServerConfig{
		PremineAccts: []*SrvAccount{},
		JsonRPCPort:  8545,
	}

	for addr := range c.Genesis.Alloc {
		config.PremineAccts = append(config.PremineAccts, &SrvAccount{Addr: addr})
	}

	srv := &TestServer{
		Config: config,
	}
	return srv
}

func NewTestServer(t *testing.T, callback TestServerConfigCallback) *TestServer {
	path := "polygon-sdk"

	// Sets the services to start on open ports
	config := &TestServerConfig{
		JsonRPCPort: getOpenPort(),
		GRPCPort:    getOpenPort(),
		LibP2PPort:  getOpenPort(),
	}

	// Sets the data directory
	dataDir, err := ioutil.TempDir("/tmp", "polygon-sdk-e2e-")
	if err != nil {
		t.Fatal(err)
	}

	config.DataDir = dataDir
	if callback != nil {
		callback(config)
	}

	// Build genesis file
	{
		args := []string{
			"genesis",
			// add data dir
			"--data-dir", dataDir,
		}
		// add premines
		for _, acct := range config.PremineAccts {
			args = append(args, "--premine", acct.Addr.String()+":0x"+acct.Balance.Text(16))
		}

		// add consensus flags
		switch config.Consensus {
		case ConsensusIBFT:
			args = append(args, "--consensus", "ibft")
		case ConsensusDummy:
			args = append(args, "--consensus", "dummy")
		}

		vcmd := exec.Command(path, args...)
		vcmd.Stdout = nil
		vcmd.Stderr = nil
		if err := vcmd.Run(); err != nil {
			t.Skipf("polygon-sdk genesis failed: %v", err)
		}
	}

	// Build arguments
	args := []string{
		"server",
		// add data dir
		"--data-dir", dataDir,
		// add custom chain
		"--chain", filepath.Join(dataDir, "genesis.json"),
		// enable grpc
		"--grpc", fmt.Sprintf(":%d", config.GRPCPort),
		// enable libp2p
		"--libp2p", fmt.Sprintf(":%d", config.LibP2PPort),
		// enable jsonrpc
		"--jsonrpc", fmt.Sprintf(":%d", config.JsonRPCPort),
	}

	if config.Seal {
		args = append(args, "--seal")
	}

	if config.DevMode {
		args = append(args, "--dev")
	}

	stdout := io.Writer(os.Stdout)
	stderr := io.Writer(os.Stdout)

	// Start the server
	cmd := exec.Command(path, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("err: %s", err)
	}

	srv := &TestServer{
		t:      t,
		Config: config,
		cmd:    cmd,
	}

	// wait until is ready
	for {
		if _, err := srv.Operator().GetStatus(context.Background(), &empty.Empty{}); err == nil {
			break
		}
	}
	return srv
}

// DeployContract deploys a contract with account 0 and returns the address
func (t *TestServer) DeployContract(c *testutil.Contract) (*compiler.Artifact, web3.Address) {
	solcContract, err := c.Compile()
	if err != nil {
		panic(err)
	}
	buf, err := hex.DecodeString(solcContract.Bin)
	if err != nil {
		panic(err)
	}
	receipt, err := t.SendTxn(&web3.Transaction{
		Input: buf,
	})
	if err != nil {
		panic(err)
	}
	return solcContract, receipt.ContractAddress
}

const (
	DefaultGasPrice = 1879048192 // 0x70000000
	DefaultGasLimit = 5242880    // 0x500000
)

var emptyAddr web3.Address

func (t *TestServer) SendTxn(txn *web3.Transaction) (*web3.Receipt, error) {
	client := t.JSONRPC()

	if txn.From == emptyAddr {
		txn.From = web3.Address(t.Config.PremineAccts[0].Addr)
	}
	if txn.GasPrice == 0 {
		txn.GasPrice = DefaultGasPrice
	}
	if txn.Gas == 0 {
		txn.Gas = DefaultGasLimit
	}
	hash, err := client.Eth().SendTransaction(txn)
	if err != nil {
		return nil, err
	}
	return t.WaitForReceipt(hash)
}

func (t *TestServer) WaitForReceipt(hash web3.Hash) (*web3.Receipt, error) {
	client := t.JSONRPC()

	var receipt *web3.Receipt
	var count uint64
	var err error

	for {
		receipt, err = client.Eth().GetTransactionReceipt(hash)
		if err != nil {
			if err.Error() != "not found" {
				return nil, err
			}
		}
		if receipt != nil {
			break
		}
		if count > 5 {
			return nil, fmt.Errorf("timeout")
		}
		time.Sleep(1 * time.Second)
		count++
	}
	return receipt, nil
}

func (t *TestServer) TxnTo(address web3.Address, method string) *web3.Receipt {
	sig := MethodSig(method)
	receipt, err := t.SendTxn(&web3.Transaction{
		To:    &address,
		Input: sig,
	})
	if err != nil {
		t.t.Fatal(err)
	}
	return receipt
}

// MethodSig returns the signature of a non-parametrized function
func MethodSig(name string) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write([]byte(name + "()"))
	b := h.Sum(nil)
	return b[:4]
}
