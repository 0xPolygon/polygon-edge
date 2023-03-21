package framework

import (
	"fmt"
	"io/ioutil"
	"math/big"
	"path"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/server/proto"
	txpoolProto "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"google.golang.org/grpc"
)

type TestServerConfig struct {
	Name                  string
	JSONRPCPort           int64
	GRPCPort              int64
	P2PPort               int64
	Seal                  bool
	DataDir               string
	Chain                 string
	LogLevel              string
	Relayer               bool
	NumBlockConfirmations uint64
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

func (t *TestServer) DataDir() string {
	return t.config.DataDir
}

func (t *TestServer) TxnPoolOperator() txpoolProto.TxnPoolOperatorClient {
	conn, err := grpc.Dial(t.GrpcAddr(), grpc.WithInsecure())
	if err != nil {
		t.t.Fatal(err)
	}

	return txpoolProto.NewTxnPoolOperatorClient(conn)
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
		t:             t,
		clusterConfig: clusterConfig,
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
		"--" + polybftsecrets.AccountDirFlag, config.DataDir,
		// add custom chain
		"--chain", config.Chain,
		// enable p2p port
		"--libp2p", fmt.Sprintf(":%d", config.P2PPort),
		// grpc port
		"--grpc-address", fmt.Sprintf("localhost:%d", config.GRPCPort),
		// enable jsonrpc
		"--jsonrpc", fmt.Sprintf(":%d", config.JSONRPCPort),
		// minimal number of child blocks required for the parent block to be considered final
		"--num-block-confirmations", strconv.FormatUint(config.NumBlockConfirmations, 10),
	}

	if len(config.LogLevel) > 0 {
		args = append(args, "--log-level", config.LogLevel)
	} else {
		args = append(args, "--log-level", "DEBUG")
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
		"--" + polybftsecrets.AccountDirFlag, t.config.DataDir,
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
		"--" + polybftsecrets.AccountDirFlag, t.config.DataDir,
		"--jsonrpc", t.JSONRPCAddr(),
		"--amount", strconv.FormatUint(amount, 10),
		"--self",
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("stake"))
}

// RegisterValidator is a wrapper function which registers new validator with given balance and stake
func (t *TestServer) RegisterValidator(secrets string, stake string) error {
	args := []string{
		"polybft",
		"register-validator",
		"--" + polybftsecrets.AccountDirFlag, path.Join(t.clusterConfig.TmpDir, secrets),
		"--jsonrpc", t.JSONRPCAddr(),
	}

	if stake != "" {
		args = append(args, "--stake", stake)
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("register-validator"))
}

// WhitelistValidator invokes whitelist-validator helper CLI command,
// which sends whitelist transaction to ChildValidatorSet
func (t *TestServer) WhitelistValidator(address, secrets string) error {
	args := []string{
		"polybft",
		"whitelist-validator",
		"--" + polybftsecrets.AccountDirFlag, path.Join(t.clusterConfig.TmpDir, secrets),
		"--address", address,
		"--jsonrpc", t.JSONRPCAddr(),
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("whitelist-validator"))
}

// Delegate delegates given amount by the account in secrets to validatorAddr validator
func (t *TestServer) Delegate(amount uint64, secrets string, validatorAddr ethgo.Address) error {
	args := []string{
		"polybft",
		"stake",
		"--" + polybftsecrets.AccountDirFlag, secrets,
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
		"--" + polybftsecrets.AccountDirFlag, secrets,
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
		"--" + polybftsecrets.AccountDirFlag, secrets,
		"--to", recipient.String(),
		"--jsonrpc", t.JSONRPCAddr(),
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("withdrawal"))
}

// HasValidatorSealed checks whether given validator has signed at least single block for the given range of blocks
func (t *TestServer) HasValidatorSealed(firstBlock, lastBlock uint64, validators polybft.AccountSet,
	validatorAddr ethgo.Address) (bool, error) {
	rpcClient := t.JSONRPC()
	for i := firstBlock + 1; i <= lastBlock; i++ {
		block, err := rpcClient.Eth().GetBlockByNumber(ethgo.BlockNumber(i), false)
		if err != nil {
			return false, err
		}

		extra, err := polybft.GetIbftExtra(block.ExtraData)
		if err != nil {
			return false, err
		}

		signers, err := validators.GetFilteredValidators(extra.Parent.Bitmap)
		if err != nil {
			return false, err
		}

		if signers.ContainsAddress(types.Address(validatorAddr)) {
			return true, nil
		}
	}

	return false, nil
}

func (t *TestServer) WaitForNonZeroBalance(address ethgo.Address, dur time.Duration) (*big.Int, error) {
	timer := time.NewTimer(dur)
	defer timer.Stop()

	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	rpcClient := t.JSONRPC()

	for {
		select {
		case <-timer.C:
			return nil, fmt.Errorf("timeout occurred while waiting for balance ")
		case <-ticker.C:
			balance, err := rpcClient.Eth().GetBalance(address, ethgo.Latest)
			if err != nil {
				return nil, fmt.Errorf("error getting balance")
			}

			if balance.Cmp(big.NewInt(0)) == 1 {
				return balance, nil
			}
		}
	}
}
