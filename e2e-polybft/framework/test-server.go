package framework

import (
	"fmt"
	"io/ioutil"
	"path"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/0xPolygon/polygon-edge/server/proto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"google.golang.org/grpc"
)

type TestServerConfig struct {
	Name        string
	JSONRPCPort int64
	GRPCPort    int64
	P2PPort     int64
	Seal        bool
	DataDir     string
	Chain       string
	LogLevel    string
	Relayer     bool
}

type TestServerConfigCallback func(*TestServerConfig)

const hostIP = "127.0.0.1"

var initialPortForServer = int64(12000)

func getOpenPortForServer() int64 {
	return atomic.AddInt64(&initialPortForServer, 1)
}

type TestServer struct {
	t *testing.T

	clusterConfig *TestClusterConfig
	config        *TestServerConfig
	node          *node
}

func (t *TestServer) GrpcAddr() string {
	return fmt.Sprintf("%s:%d", hostIP, t.config.GRPCPort)
}

func (t *TestServer) JSONRPCAddr() string {
	return fmt.Sprintf("http://%s:%d", hostIP, t.config.JSONRPCPort)
}

func (t *TestServer) JSONRPC() *jsonrpc.Client {
	clt, err := jsonrpc.NewClient(t.JSONRPCAddr())
	if err != nil {
		t.t.Fatal(err)
	}

	return clt
}

func (t *TestServer) Conn() proto.SystemClient {
	conn, err := grpc.Dial(t.GrpcAddr(), grpc.WithInsecure())
	if err != nil {
		t.t.Fatal(err)
	}

	return proto.NewSystemClient(conn)
}

func NewTestServer(t *testing.T, clusterConfig *TestClusterConfig, callback TestServerConfigCallback) *TestServer {
	t.Helper()

	config := &TestServerConfig{
		Name:        uuid.New().String(),
		JSONRPCPort: getOpenPortForServer(),
		GRPCPort:    getOpenPortForServer(),
		P2PPort:     getOpenPortForServer(),
	}

	if callback != nil {
		callback(config)
	}

	if config.DataDir == "" {
		dataDir, err := ioutil.TempDir("/tmp", "edge-e2e-")
		assert.NoError(t, err)

		config.DataDir = dataDir
	}

	srv := &TestServer{
		clusterConfig: clusterConfig,
		t:             t,
		config:        config,
	}
	srv.Start()

	return srv
}

func (t *TestServer) isRunning() bool {
	return t.node != nil
}

func (t *TestServer) Start() {
	config := t.config

	// Build arguments
	args := []string{
		"server",
		// add data dir
		"--data-dir", config.DataDir,
		// add custom chain
		"--chain", config.Chain,
		// enable p2p port
		"--libp2p", fmt.Sprintf(":%d", config.P2PPort),
		// grpc port
		"--grpc-address", fmt.Sprintf("localhost:%d", config.GRPCPort),
		// enable jsonrpc
		"--jsonrpc", fmt.Sprintf(":%d", config.JSONRPCPort),
	}

	if len(config.LogLevel) > 0 {
		args = append(args, "--log-level", config.LogLevel)
	}

	if config.Seal {
		args = append(args, "--seal")
	}

	if config.Relayer {
		args = append(args, "--relayer")
	}

	// Start the server
	stdout := t.clusterConfig.GetStdout(t.config.Name)

	node, err := newNode(t.clusterConfig.Binary, args, stdout)
	if err != nil {
		t.t.Fatal(err)
	}

	t.node = node
}

func (t *TestServer) Stop() {
	if err := t.node.Stop(); err != nil {
		t.t.Fatal(err)
	}

	t.node = nil
}

// Stake stakes given amount to validator account encapsulated by given server instance
func (t *TestServer) Stake(amount uint64) error {
	args := []string{
		"polybft",
		"stake",
		"--account", t.config.DataDir,
		"--jsonrpc", t.JSONRPCAddr(),
		"--amount", strconv.FormatUint(amount, 10),
		"--self",
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("stake"))
}

// Unstake unstakes given amount from validator account encapsulated by given server instance
func (t *TestServer) Unstake(amount uint64) error {
	args := []string{
		"polybft",
		"unstake",
		"--account", t.config.DataDir,
		"--jsonrpc", t.JSONRPCAddr(),
		"--amount", strconv.FormatUint(amount, 10),
		"--self",
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("stake"))
}

// RegisterValidator is a wrapper function which registers new validator with given balance and stake
func (t *TestServer) RegisterValidator(secrets string, balance string, stake string) error {
	args := []string{
		"polybft",
		"register-validator",
		"--data-dir", path.Join(t.clusterConfig.TmpDir, secrets),
		"--registrator-data-dir", path.Join(t.clusterConfig.TmpDir, "test-chain-1"),
		"--jsonrpc", t.JSONRPCAddr(),
		"--balance", balance,
		"--stake", stake,
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("register-validator"))
}

// Delegate delegates given amount by the account in secrets to validatorAddr validator
func (t *TestServer) Delegate(amount uint64, secrets string, validatorAddr ethgo.Address) error {
	args := []string{
		"polybft",
		"stake",
		"--account", secrets,
		"--jsonrpc", t.JSONRPCAddr(),
		"--delegate", validatorAddr.String(),
		"--amount", strconv.FormatUint(amount, 10),
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("delegation"))
}

// Undelegate undelegates given amount by the account in secrets from validatorAddr validator
func (t *TestServer) Undelegate(amount uint64, secrets string, validatorAddr ethgo.Address) error {
	args := []string{
		"polybft",
		"unstake",
		"--account", secrets,
		"--undelegate", validatorAddr.String(),
		"--amount", strconv.FormatUint(amount, 10),
		"--jsonrpc", t.JSONRPCAddr(),
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("delegation"))
}

// Withdraw withdraws available balance to provided recipient address
func (t *TestServer) Withdraw(secrets string, recipient ethgo.Address) error {
	args := []string{
		"polybft",
		"withdraw",
		"--account", secrets,
		"--to", recipient.String(),
		"--jsonrpc", t.JSONRPCAddr(),
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("withdrawal"))
}
