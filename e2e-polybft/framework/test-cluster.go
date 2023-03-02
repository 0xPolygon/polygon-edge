package framework

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
)

const (
	// envE2ETestsEnabled signal whether the e2e tests will run
	envE2ETestsEnabled = "E2E_TESTS"

	// envLogsEnabled signal whether the output of the nodes get piped to a log file
	envLogsEnabled = "E2E_LOGS"

	// envLogLevel specifies log level of each node
	envLogLevel = "E2E_LOG_LEVEL"

	// envStdoutEnabled signal whether the output of the nodes get piped to stdout
	envStdoutEnabled = "E2E_STDOUT"

	// property based tests enabled
	envPropertyBaseTestEnabled = "E2E_PROPERTY_TESTS"
)

const (
	// prefix for validator directory
	defaultValidatorPrefix = "test-chain-"
)

var startTime int64

func init() {
	startTime = time.Now().UnixMilli()
}

func resolveBinary() string {
	bin := os.Getenv("EDGE_BINARY")
	if bin != "" {
		return bin
	}
	// fallback
	return "polygon-edge"
}

type TestClusterConfig struct {
	t *testing.T

	Name              string
	Premine           []string // address[:amount]
	PremineValidators string
	HasBridge         bool
	BootnodeCount     int
	NonValidatorCount int
	WithLogs          bool
	WithStdout        bool
	LogsDir           string
	TmpDir            string
	BlockGasLimit     uint64
	ValidatorPrefix   string
	Binary            string
	ValidatorSetSize  uint64
	EpochSize         int
	EpochReward       int
	PropertyBaseTests bool
	SecretsCallback   func([]types.Address, *TestClusterConfig)

	NumBlockConfirmations uint64

	logsDirOnce sync.Once
}

func (c *TestClusterConfig) Dir(name string) string {
	return filepath.Join(c.TmpDir, name)
}

func (c *TestClusterConfig) GetStdout(name string, custom ...io.Writer) io.Writer {
	writers := []io.Writer{}

	if c.WithLogs {
		c.logsDirOnce.Do(func() {
			c.initLogsDir()
		})

		f, err := os.OpenFile(filepath.Join(c.LogsDir, name+".log"), os.O_RDWR|os.O_APPEND|os.O_CREATE, 0600)
		if err != nil {
			c.t.Fatal(err)
		}

		writers = append(writers, f)

		c.t.Cleanup(func() {
			err = f.Close()
			if err != nil {
				c.t.Logf("Failed to close file. Error: %s", err)
			}
		})
	}

	if c.WithStdout {
		writers = append(writers, os.Stdout)
	}

	if len(custom) > 0 {
		writers = append(writers, custom...)
	}

	if len(writers) == 0 {
		return io.Discard
	}

	return io.MultiWriter(writers...)
}

func (c *TestClusterConfig) initLogsDir() {
	logsDir := path.Join("..", fmt.Sprintf("e2e-logs-%d", startTime), c.t.Name())

	if err := common.CreateDirSafe(logsDir, 0750); err != nil {
		c.t.Fatal(err)
	}

	c.t.Logf("logs enabled for e2e test: %s", logsDir)
	c.LogsDir = logsDir
}

type TestCluster struct {
	Config      *TestClusterConfig
	Servers     []*TestServer
	Bridge      *TestBridge
	initialPort int64

	once         sync.Once
	failCh       chan struct{}
	executionErr error
}

type ClusterOption func(*TestClusterConfig)

func WithPremine(addresses ...types.Address) ClusterOption {
	return func(h *TestClusterConfig) {
		for _, a := range addresses {
			h.Premine = append(h.Premine, a.String())
		}
	}
}

func WithPremineValidators(premineBalance string) ClusterOption {
	return func(h *TestClusterConfig) {
		h.PremineValidators = premineBalance
	}
}

func WithSecretsCallback(fn func([]types.Address, *TestClusterConfig)) ClusterOption {
	return func(h *TestClusterConfig) {
		h.SecretsCallback = fn
	}
}

func WithBridge() ClusterOption {
	return func(h *TestClusterConfig) {
		h.HasBridge = true
	}
}

func WithNonValidators(num int) ClusterOption {
	return func(h *TestClusterConfig) {
		h.NonValidatorCount = num
	}
}

func WithValidatorSnapshot(validatorsLen uint64) ClusterOption {
	return func(h *TestClusterConfig) {
		h.ValidatorSetSize = validatorsLen
	}
}

func WithBootnodeCount(cnt int) ClusterOption {
	return func(h *TestClusterConfig) {
		h.BootnodeCount = cnt
	}
}

func WithEpochSize(epochSize int) ClusterOption {
	return func(h *TestClusterConfig) {
		h.EpochSize = epochSize
	}
}

func WithEpochReward(epochReward int) ClusterOption {
	return func(h *TestClusterConfig) {
		h.EpochReward = epochReward
	}
}

func WithBlockGasLimit(blockGasLimit uint64) ClusterOption {
	return func(h *TestClusterConfig) {
		h.BlockGasLimit = blockGasLimit
	}
}

func WithPropertyBaseTests(propertyBaseTests bool) ClusterOption {
	return func(h *TestClusterConfig) {
		h.PropertyBaseTests = propertyBaseTests
	}
}

func WithNumBlockConfirmations(numBlockConfirmations uint64) ClusterOption {
	return func(h *TestClusterConfig) {
		h.NumBlockConfirmations = numBlockConfirmations
	}
}

func isTrueEnv(e string) bool {
	return strings.ToLower(os.Getenv(e)) == "true"
}

func NewTestCluster(t *testing.T, validatorsCount int, opts ...ClusterOption) *TestCluster {
	t.Helper()

	var err error

	config := &TestClusterConfig{
		t:                 t,
		WithLogs:          isTrueEnv(envLogsEnabled),
		WithStdout:        isTrueEnv(envStdoutEnabled),
		Binary:            resolveBinary(),
		EpochSize:         10,
		EpochReward:       1,
		BlockGasLimit:     1e7, // 10M
		PremineValidators: command.DefaultPremineBalance,
	}

	if config.ValidatorPrefix == "" {
		config.ValidatorPrefix = defaultValidatorPrefix
	}

	for _, opt := range opts {
		opt(config)
	}

	if !config.PropertyBaseTests && !isTrueEnv(envE2ETestsEnabled) ||
		config.PropertyBaseTests && !isTrueEnv(envPropertyBaseTestEnabled) {
		t.Skip("Integration tests are disabled.")
	}

	config.TmpDir, err = os.MkdirTemp("/tmp", "e2e-polybft-")
	require.NoError(t, err)

	cluster := &TestCluster{
		Servers:     []*TestServer{},
		Config:      config,
		initialPort: 30300,
		failCh:      make(chan struct{}),
		once:        sync.Once{},
	}

	{
		// run init accounts
		addresses, err := cluster.InitSecrets(cluster.Config.ValidatorPrefix, validatorsCount)
		require.NoError(t, err)

		if cluster.Config.SecretsCallback != nil {
			cluster.Config.SecretsCallback(addresses, cluster.Config)
		}
	}

	manifestPath := path.Join(config.TmpDir, "manifest.json")
	args := []string{
		"manifest",
		"--path", manifestPath,
		"--validators-path", config.TmpDir,
		"--validators-prefix", cluster.Config.ValidatorPrefix,
		"--premine-validators", cluster.Config.PremineValidators,
	}
	// run manifest file creation
	require.NoError(t, cluster.cmdRun(args...))

	if cluster.Config.HasBridge {
		// start bridge
		cluster.Bridge, err = NewTestBridge(t, cluster.Config)
		require.NoError(t, err)
	}

	// in case no validators are specified in opts, all nodes will be validators
	if cluster.Config.ValidatorSetSize == 0 {
		cluster.Config.ValidatorSetSize = uint64(validatorsCount)
	}

	if cluster.Config.HasBridge {
		err := cluster.Bridge.deployRootchainContracts(manifestPath)
		require.NoError(t, err)

		err = cluster.Bridge.fundRootchainValidators()
		require.NoError(t, err)
	}

	{
		// run genesis configuration population
		args := []string{
			"genesis",
			"--manifest", manifestPath,
			"--consensus", "polybft",
			"--dir", path.Join(config.TmpDir, "genesis.json"),
			"--block-gas-limit", strconv.FormatUint(cluster.Config.BlockGasLimit, 10),
			"--epoch-size", strconv.Itoa(cluster.Config.EpochSize),
			"--epoch-reward", strconv.Itoa(cluster.Config.EpochReward),
			"--premine", "0x0000000000000000000000000000000000000000",
		}

		if len(cluster.Config.Premine) != 0 {
			for _, premine := range cluster.Config.Premine {
				args = append(args, "--premine", premine)
			}
		}

		if cluster.Config.HasBridge {
			rootchainIP, err := helper.ReadRootchainIP()
			require.NoError(t, err)
			args = append(args, "--bridge-json-rpc", rootchainIP)
		}

		validators, err := genesis.ReadValidatorsByPrefix(
			cluster.Config.TmpDir, cluster.Config.ValidatorPrefix)
		require.NoError(t, err)

		if cluster.Config.BootnodeCount > 0 {
			cnt := cluster.Config.BootnodeCount
			if len(validators) < cnt {
				cnt = len(validators)
			}

			for i := 0; i < cnt; i++ {
				maddr := fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s",
					"127.0.0.1", cluster.initialPort+int64(i+1), validators[i].NodeID)
				args = append(args, "--bootnode", maddr)
			}
		}

		if cluster.Config.ValidatorSetSize > 0 {
			args = append(args, "--validator-set-size", fmt.Sprint(cluster.Config.ValidatorSetSize))
		}

		// run cmd init-genesis with all the arguments
		err = cluster.cmdRun(args...)
		require.NoError(t, err)
	}

	for i := 1; i <= int(cluster.Config.ValidatorSetSize); i++ {
		cluster.InitTestServer(t, i, true, cluster.Config.HasBridge && i == 1 /* relayer */)
	}

	for i := 1; i <= cluster.Config.NonValidatorCount; i++ {
		offsetIndex := i + int(cluster.Config.ValidatorSetSize)
		cluster.InitTestServer(t, offsetIndex, false, false /* relayer */)
	}

	return cluster
}

func (c *TestCluster) InitTestServer(t *testing.T, i int, isValidator bool, relayer bool) {
	t.Helper()

	logLevel := os.Getenv(envLogLevel)
	dataDir := c.Config.Dir(c.Config.ValidatorPrefix + strconv.Itoa(i))

	srv := NewTestServer(t, c.Config, func(config *TestServerConfig) {
		config.DataDir = dataDir
		config.Seal = isValidator
		config.Chain = c.Config.Dir("genesis.json")
		config.P2PPort = c.getOpenPort()
		config.LogLevel = logLevel
		config.Relayer = relayer
		config.NumBlockConfirmations = c.Config.NumBlockConfirmations
	})

	// watch the server for stop signals. It is important to fix the specific
	// 'node' reference since 'TestServer' creates a new one if restarted.
	go func(node *node) {
		<-node.Wait()

		if !node.ExitResult().Signaled {
			c.Fail(fmt.Errorf("server at dir '%s' has stopped unexpectedly", dataDir))
		}
	}(srv.node)

	c.Servers = append(c.Servers, srv)
}

func (c *TestCluster) cmdRun(args ...string) error {
	return runCommand(c.Config.Binary, args, c.Config.GetStdout(args[0]))
}

func (c *TestCluster) Fail(err error) {
	c.once.Do(func() {
		c.executionErr = err
		close(c.failCh)
	})
}

func (c *TestCluster) Stop() {
	if c.Bridge != nil {
		c.Bridge.Stop()
	}

	for _, srv := range c.Servers {
		if srv.isRunning() {
			srv.Stop()
		}
	}
}

func (c *TestCluster) Stats(t *testing.T) {
	t.Helper()

	for index, i := range c.Servers {
		if !i.isRunning() {
			continue
		}

		num, err := i.JSONRPC().Eth().BlockNumber()
		t.Log("Stats node", index, "err", err, "block", num, "validator", i.config.Seal)
	}
}

func (c *TestCluster) WaitUntil(dur time.Duration, handler func() bool) error {
	timer := time.NewTimer(dur)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout")
		case <-c.failCh:
			return c.executionErr
		case <-time.After(2 * time.Second):
		}

		if handler() {
			return nil
		}
	}
}

func (c *TestCluster) WaitForBlock(n uint64, timeout time.Duration) error {
	timer := time.NewTimer(timeout)

	ok := false
	for !ok {
		select {
		case <-timer.C:
			return fmt.Errorf("wait for block timeout")
		case <-time.After(2 * time.Second):
		}

		ok = true

		for _, i := range c.Servers {
			if !i.isRunning() {
				continue
			}

			num, err := i.JSONRPC().Eth().BlockNumber()

			if err != nil || num < n {
				ok = false

				break
			}
		}
	}

	return nil
}

// WaitForGeneric waits until all running servers returns true from fn callback or timeout defined by dur occurs
func (c *TestCluster) WaitForGeneric(dur time.Duration, fn func(*TestServer) bool) error {
	return c.WaitUntil(dur, func() bool {
		for _, srv := range c.Servers {
			// query only running servers
			if srv.isRunning() && !fn(srv) {
				return false
			}
		}

		return true
	})
}

func (c *TestCluster) getOpenPort() int64 {
	c.initialPort++

	return c.initialPort
}

// runCommand executes command with given arguments
func runCommand(binary string, args []string, stdout io.Writer) error {
	var stdErr bytes.Buffer

	cmd := exec.Command(binary, args...)
	cmd.Stderr = &stdErr
	cmd.Stdout = stdout

	if err := cmd.Run(); err != nil {
		if stdErr.Len() > 0 {
			return fmt.Errorf("failed to execute command: %s", stdErr.String())
		}

		return fmt.Errorf("failed to execute command: %w", err)
	}

	if stdErr.Len() > 0 {
		return fmt.Errorf("error during command execution: %s", stdErr.String())
	}

	return nil
}

// InitSecrets initializes account(s) secrets with given prefix.
// (secrets are being stored in the temp directory created by given e2e test execution)
func (c *TestCluster) InitSecrets(prefix string, count int) ([]types.Address, error) {
	var b bytes.Buffer

	args := []string{
		"polybft-secrets",
		"--data-dir", path.Join(c.Config.TmpDir, prefix),
		"--num", strconv.Itoa(count),
		"--insecure",
	}
	stdOut := c.Config.GetStdout("polybft-secrets", &b)

	if err := runCommand(c.Config.Binary, args, stdOut); err != nil {
		return nil, err
	}

	re := regexp.MustCompile("\\(address\\) = 0x([a-fA-F0-9]+)")
	parsed := re.FindAllStringSubmatch(b.String(), -1)
	result := make([]types.Address, len(parsed))

	for i, v := range parsed {
		result[i] = types.StringToAddress(v[1])
	}

	return result, nil
}
