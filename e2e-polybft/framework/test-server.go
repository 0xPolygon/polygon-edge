package framework

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	polybftsecrets "github.com/0xPolygon/polygon-edge/command/secrets/init"
	validatorHelper "github.com/0xPolygon/polygon-edge/command/validator/helper"
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
	TLSCertFile           string
	TLSKeyFile            string
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
	if t.config.TLSCertFile != "" && t.config.TLSKeyFile != "" {
		return fmt.Sprintf("https://localhost:%d", t.config.JSONRPCPort)
	} else {
		return fmt.Sprintf("http://%s:%d", hostIP, t.config.JSONRPCPort)
	}
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
		// TLS certificate file
		"--tls-cert-file", config.TLSCertFile,
		// TLS key file
		"--tls-key-file", config.TLSKeyFile,
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
func (t *TestServer) RootchainFund(amount *big.Int) error {
	return t.RootchainFundFor([]types.Address{t.address}, []*big.Int{amount})
}

// RootchainFundFor funds given account on the rootchain
func (t *TestServer) RootchainFundFor(accounts []types.Address, amounts []*big.Int) error {
	if len(accounts) != len(amounts) {
		return errors.New("same size for accounts and amounts must be provided to the rootchain funding")
	}

	args := []string{
		"bridge",
		"fund",
		"--json-rpc", t.BridgeJSONRPCAddr(),
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
func (t *TestServer) Stake(stakeTokenAddr types.Address, amount *big.Int) error {
	args := []string{
		"validator",
		"stake",
		"--jsonrpc", t.JSONRPCAddr(),
		"--" + polybftsecrets.AccountDirFlag, t.config.DataDir,
		"--amount", amount.String(),
	}

	if stakeTokenAddr != types.ZeroAddress {
		args = append(args, "--stake-token", stakeTokenAddr.String())
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("stake"))
}

// Unstake unstakes given amount from validator account encapsulated by given server instance
func (t *TestServer) Unstake(amount *big.Int) error {
	args := []string{
		"validator",
		"unstake",
		"--" + polybftsecrets.AccountDirFlag, t.config.DataDir,
		"--jsonrpc", t.JSONRPCAddr(),
		"--amount", amount.String(),
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("unstake"))
}

// RegisterValidator is a wrapper function which registers new validator
func (t *TestServer) RegisterValidatorWithStake(amount *big.Int) error {
	args := []string{
		"validator",
		"register-validator",
		"--" + polybftsecrets.AccountDirFlag, t.DataDir(),
		"--jsonrpc", t.JSONRPCAddr(),
		"--amount", amount.String(),
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("validator"))
}

// WhitelistValidators invokes whitelist-validators helper CLI command that whitelists validators
func (t *TestServer) WhitelistValidators(addresses []string) error {
	args := []string{
		"validator",
		"whitelist-validators",
		"--" + polybftsecrets.AccountDirFlag, t.config.DataDir,
		"--jsonrpc", t.JSONRPCAddr(),
	}
	for _, addr := range addresses {
		args = append(args, "--addresses", addr)
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("validator"))
}

// MintERC20Token mints given amounts of native erc20 token on blade to given addresses
func (t *TestServer) MintERC20Token(addresses []string, amounts []*big.Int, erc20Token types.Address) error {
	acc, err := validatorHelper.GetAccountFromDir(t.DataDir())
	if err != nil {
		return err
	}

	rawKey, err := acc.Ecdsa.MarshallPrivateKey()
	if err != nil {
		return err
	}

	args := []string{
		"mint-erc20",
		"--jsonrpc", t.JSONRPCAddr(),
		"--erc20-token", erc20Token.String(),
		"--private-key", hex.EncodeToString(rawKey),
	}

	for _, addr := range addresses {
		args = append(args, "--addresses", addr)
	}

	for _, amount := range amounts {
		args = append(args, "--amounts", amount.String())
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("mint-erc20"))
}

// WitdhrawStake withdraws given amount of stake back to the validator address
func (t *TestServer) WitdhrawStake() error {
	args := []string{
		"validator",
		"withdraw",
		"--" + polybftsecrets.AccountDirFlag, t.config.DataDir,
		"--jsonrpc", t.JSONRPCAddr(),
	}

	return runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("withdraw"))
}

// WithdrawRewards withdraws pending rewards for given validator on RewardPool contract
func (t *TestServer) WithdrawRewards() error {
	args := []string{
		"validator",
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
