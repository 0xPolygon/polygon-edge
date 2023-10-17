package framework

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	rootHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/server/proto"
	txpoolProto "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"google.golang.org/grpc"
)

type TestServerConfig struct {
	Name                  string
	JSONRPCPort           int64
	GRPCPort              int64
	P2PPort               int64
	Validator             bool
	DataDir               string
	Chain                 string
	LogLevel              string
	Relayer               bool
	NumBlockConfirmations uint64
	BridgeJSONRPC         string
}

type TestServerConfigCallback func(*TestServerConfig)

const hostIP = "127.0.0.1"

var initialPortForServer = int64(12000)

func getOpenPortForServer() int64 {
	return atomic.AddInt64(&initialPortForServer, 1)
}

type TestServer struct {
	t *testing.T

	address       types.Address
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

func (t *TestServer) BridgeJSONRPCAddr() string {
	return t.config.BridgeJSONRPC
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

func NewTestServer(t *testing.T, clusterConfig *TestClusterConfig,
	bridgeJSONRPC string, callback TestServerConfigCallback) *TestServer {
	t.Helper()

	config := &TestServerConfig{
		Name:          uuid.New().String(),
		JSONRPCPort:   getOpenPortForServer(),
		GRPCPort:      getOpenPortForServer(),
		P2PPort:       getOpenPortForServer(),
		BridgeJSONRPC: bridgeJSONRPC,
	}

	if callback != nil {
		callback(config)
	}

	if config.DataDir == "" {
		dataDir, err := os.MkdirTemp("/tmp", "edge-e2e-")
		require.NoError(t, err)

		config.DataDir = dataDir
	}

	secretsManager, err := polybftsecrets.GetSecretsManager(config.DataDir, "", true)
	require.NoError(t, err)

	key, err := wallet.GetEcdsaFromSecret(secretsManager)
	require.NoError(t, err)

	srv := &TestServer{
		t:             t,
		clusterConfig: clusterConfig,
		address:       types.Address(key.Address()),
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

// RootchainFund funds given validator account on the rootchain
func (t *TestServer) RootchainFund(stakeToken types.Address, amount *big.Int) error {
	return t.RootchainFundFor([]types.Address{t.address}, []*big.Int{amount}, stakeToken)
}

// RootchainFundFor funds given account on the rootchain
func (t *TestServer) RootchainFundFor(accounts []types.Address, amounts []*big.Int, stakeToken types.Address) error {
	if len(accounts) != len(amounts) {
		return errors.New("same size for accounts and amounts must be provided to the rootchain funding")
	}

	args := []string{
		"rootchain",
		"fund",
		"--json-rpc", t.BridgeJSONRPCAddr(),
		"--stake-token", stakeToken.String(),
		"--mint",
	}

	for i := 0; i < len(accounts); i++ {
		args = append(args, "--addresses", accounts[i].String())
		args = append(args, "--amounts", amounts[i].String())
	}

	if err := runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("bridge")); err != nil {
		acctAddrs := make([]string, len(accounts))
		for i, acc := range accounts {
			acctAddrs[i] = acc.String()
		}

		return fmt.Errorf("failed to fund accounts (%s) on the rootchain: %w", strings.Join(acctAddrs, ","), err)
	}

	return nil
}

// Stake stakes given amount to validator account encapsulated by given server instance
func (t *TestServer) Stake(polybftConfig polybft.PolyBFTConfig, amount *big.Int) error {
	args := []string{
		"polybft",
		"stake",
		"--jsonrpc", t.BridgeJSONRPCAddr(),
		"--stake-manager", polybftConfig.Bridge.StakeManagerAddr.String(),
		"--" + polybftsecrets.AccountDirFlag, t.config.DataDir,
		"--amount", amount.String(),
		"--supernet-id", strconv.FormatInt(polybftConfig.SupernetID, 10),
		"--stake-token", polybftConfig.Bridge.StakeTokenAddr.String(),
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("stake"))
}

// Unstake unstakes given amount from validator account encapsulated by given server instance
func (t *TestServer) Unstake(amount *big.Int) error {
	args := []string{
		"polybft",
		"unstake",
		"--" + polybftsecrets.AccountDirFlag, t.config.DataDir,
		"--jsonrpc", t.JSONRPCAddr(),
		"--amount", amount.String(),
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("unstake"))
}

// RegisterValidator is a wrapper function which registers new validator on a root chain
func (t *TestServer) RegisterValidator(supernetManagerAddr types.Address) error {
	args := []string{
		"polybft",
		"register-validator",
		"--jsonrpc", t.BridgeJSONRPCAddr(),
		"--supernet-manager", supernetManagerAddr.String(),
		"--" + polybftsecrets.AccountDirFlag, t.DataDir(),
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("bridge"))
}

// WhitelistValidators invokes whitelist-validators helper CLI command,
// that whitelists validators on the root chain
func (t *TestServer) WhitelistValidators(addresses []string, supernetManager types.Address) error {
	args := []string{
		"polybft",
		"whitelist-validators",
		"--private-key", rootHelper.TestAccountPrivKey,
		"--jsonrpc", t.BridgeJSONRPCAddr(),
		"--supernet-manager", supernetManager.String(),
	}
	for _, addr := range addresses {
		args = append(args, "--addresses", addr)
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("bridge"))
}

// WithdrawChildChain withdraws available balance from child chain
func (t *TestServer) WithdrawChildChain() error {
	args := []string{
		"polybft",
		"withdraw-child",
		"--" + polybftsecrets.AccountDirFlag, t.config.DataDir,
		"--jsonrpc", t.JSONRPCAddr(),
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("withdraw-child"))
}

// WithdrawRootChain withdraws available balance from root chain
func (t *TestServer) WithdrawRootChain(recipient string, amount *big.Int,
	stakeManager ethgo.Address, bridgeJSONRPC string) error {
	args := []string{
		"polybft",
		"withdraw-root",
		"--" + polybftsecrets.AccountDirFlag, t.config.DataDir,
		"--to", recipient,
		"--amount", amount.String(),
		"--stake-manager", stakeManager.String(),
		"--jsonrpc", bridgeJSONRPC,
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("withdraw-root"))
}

// WithdrawRewards withdraws pending rewards for given validator on RewardPool contract
func (t *TestServer) WithdrawRewards() error {
	args := []string{
		"polybft",
		"withdraw-rewards",
		"--" + polybftsecrets.AccountDirFlag, t.config.DataDir,
		"--jsonrpc", t.JSONRPCAddr(),
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("withdraw-rewards"))
}

// HasValidatorSealed checks whether given validator has signed at least single block for the given range of blocks
func (t *TestServer) HasValidatorSealed(firstBlock, lastBlock uint64, validators validator.AccountSet,
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
