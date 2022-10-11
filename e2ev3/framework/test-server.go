package framework

import (
	"fmt"
	"io/ioutil"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/ethgo/jsonrpc"
)

type TestServerConfig struct {
	Name        string
	JsonRPCPort int64
	GRPCPort    int64
	P2PPort     int64
	Seal        bool
	DataDir     string
	Chain       string
	Password    string
	LogLevel    string
}

type TestServerConfigCallback func(*TestServerConfig)

const hostIp = "127.0.0.1"

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
	return fmt.Sprintf("%s:%d", hostIp, t.config.GRPCPort)
}

func (t *TestServer) JsonRPCAddr() string {
	return fmt.Sprintf("http://%s:%d", hostIp, t.config.JsonRPCPort)
}

func (t *TestServer) JSONRPC() *jsonrpc.Client {
	clt, err := jsonrpc.NewClient(t.JsonRPCAddr())
	if err != nil {
		t.t.Fatal(err)
	}
	return clt
}

// func (t *TestServer) Conn() proto.BorClient {
// 	conn, err := grpc.Dial(t.GrpcAddr(), grpc.WithInsecure())
// 	if err != nil {
// 		t.t.Fatal(err)
// 	}
// 	return proto.NewBorClient(conn)
// }

func NewTestServer(t *testing.T, clusterConfig *TestClusterConfig, callback TestServerConfigCallback) *TestServer {
	config := &TestServerConfig{
		Name:        uuid.New().String(),
		JsonRPCPort: getOpenPortForServer(),
		GRPCPort:    getOpenPortForServer(),
		P2PPort:     getOpenPortForServer(),
	}

	if callback != nil {
		callback(config)
	}

	if config.DataDir == "" {
		dataDir, err := ioutil.TempDir("/tmp", "polygon-sdk-e2e-")
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
		"--datadir", config.DataDir,
		// add custom chain
		"--chain", config.Chain,
		// enable p2p port
		"--port", fmt.Sprintf("%d", config.P2PPort),
		// grpc port
		"--grpc.addr", fmt.Sprintf("localhost:%d", config.GRPCPort),
		// enable jsonrpc
		"--jsonrpc.modules", "eth",
		"--http", "--http.port", fmt.Sprintf("%d", config.JsonRPCPort),
	}

	if len(config.LogLevel) > 0 {
		args = append(args, "--log-level", config.LogLevel)
	}

	if config.Seal {
		args = append(args, "--mine")
		args = append(args, "--miner.password", config.Password)
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
