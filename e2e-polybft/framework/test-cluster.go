package framework

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
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

	// accountPassword is the default account password
	accountPassword = "qwerty"
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

func createAccountPasswordFile(t *testing.T) (string, func()) {
	t.Helper()

	pwdFile, err := os.CreateTemp("", "e2e-polybft")
	require.NoError(t, err)

	_, err = pwdFile.WriteString(accountPassword)
	require.NoError(t, err)
	require.NoError(t, pwdFile.Close())

	return pwdFile.Name(), func() {
		err = os.Remove(pwdFile.Name())
		require.NoError(t, err)
	}
}

type TestClusterConfig struct {
	t *testing.T

	Name              string
	Premine           []common.Address
	HasBridge         bool
	NonValidatorCount int
	WithLogs          bool
	WithStdout        bool
	LogsDir           string
	TmpDir            string
	Binary            string
	ValidatorSetSize  uint64

	logsDirOnce sync.Once
}

func (c *TestClusterConfig) Dir(name string) string {
	return filepath.Join(c.TmpDir, name)
}

func (c *TestClusterConfig) GetStdout(name string) io.Writer {
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

	if len(writers) == 0 {
		return io.Discard
	}

	return io.MultiWriter(writers...)
}

func (c *TestClusterConfig) initLogsDir() {
	logsDir := path.Join("..", fmt.Sprintf("e2e-logs-%d", startTime), c.t.Name())

	if err := os.MkdirAll(logsDir, 0750); err != nil {
		c.t.Fatal(err)
	}

	c.t.Logf("logs enabled for e2e test: %s", logsDir)
	c.LogsDir = logsDir
}

type TestCluster struct {
	Config   *TestClusterConfig
	Servers  []*TestServer
	Bootnode *TestBootnode
	// Bridge      *TestBridge
	initialPort int64

	once         sync.Once
	failCh       chan struct{}
	executionErr error
}

type ClusterOption func(*TestClusterConfig)

func WithPremine(address common.Address) ClusterOption {
	return func(h *TestClusterConfig) {
		h.Premine = append(h.Premine, address)
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

func isTrueEnv(e string) bool {
	return strings.ToLower(os.Getenv(e)) == "true"
}

func NewTestCluster(t *testing.T, validatorsCount int, opts ...ClusterOption) *TestCluster {
	t.Helper()

	if !isTrueEnv(envE2ETestsEnabled) {
		t.Skip("Integration tests are disabled.")
	}

	tmpDir, err := os.MkdirTemp("/tmp", "e2e-polybft-")
	require.NoError(t, err)

	config := &TestClusterConfig{
		t:          t,
		WithLogs:   isTrueEnv(envLogsEnabled),
		WithStdout: isTrueEnv(envStdoutEnabled),
		TmpDir:     tmpDir,
		Binary:     resolveBinary(),
	}

	for _, opt := range opts {
		opt(config)
	}

	cluster := &TestCluster{
		Servers:     []*TestServer{},
		Config:      config,
		initialPort: 30300,
		failCh:      make(chan struct{}),
		once:        sync.Once{},
	}

	if cluster.Config.HasBridge {
		// start bridge
		// cluster.Bridge, err = NewTestBridge(t, cluster.Config)
		require.NoError(t, err)
	}

	// Create a file with account password
	//pwdFilePath, deleteFile := createAccountPasswordFile(t)
	//defer deleteFile()

	// In case no validators are specified in opts, all nodes will be validators
	if cluster.Config.ValidatorSetSize == 0 {
		cluster.Config.ValidatorSetSize = uint64(validatorsCount)
	}

	{
		// run init account
		err = cluster.cmdRun("polybft-secrets",
			"--data-dir", path.Join(tmpDir, "test-chain-"),
			"--num", strconv.Itoa(validatorsCount),
			//"--password", pwdFilePath,
		)
		require.NoError(t, err)
	}

	{
		// create genesis file
		args := []string{
			"genesis",
			"--consensus", "polybft",
			"--dir", path.Join(tmpDir, "genesis.json"),
			//"--password", pwdFilePath,
			"--contracts-path", "./../core-contracts/artifacts/contracts/",
			"--premine", "0x0000000000000000000000000000000000000000",
		}

		if len(cluster.Config.Premine) != 0 {
			for _, addr := range cluster.Config.Premine {
				args = append(args, "--premine", addr.String())
			}
		}

		if cluster.Config.HasBridge {
			args = append(args, "--bridge")
		}

		if cluster.Config.ValidatorSetSize > 0 {
			args = append(args, "--validator-set-size", fmt.Sprint(cluster.Config.ValidatorSetSize))
		}

		// run cmd init-genesis with all the arguments
		err = cluster.cmdRun(args...)
		require.NoError(t, err)
	}

	for i := 1; i <= int(cluster.Config.ValidatorSetSize); i++ {
		cluster.initTestServer(t, i, true)
	}

	for i := 1; i <= cluster.Config.NonValidatorCount; i++ {
		offsetIndex := i + int(cluster.Config.ValidatorSetSize)
		cluster.initTestServer(t, offsetIndex, false)
	}

	return cluster
}

func (c *TestCluster) initTestServer(t *testing.T, i int, isValidator bool) {
	t.Helper()

	logLevel := os.Getenv(envLogLevel)
	dataDir := c.Config.Dir("test-chain-" + strconv.Itoa(i))

	srv := NewTestServer(t, c.Config, func(config *TestServerConfig) {
		config.DataDir = dataDir
		config.Seal = isValidator
		config.Chain = c.Config.Dir("genesis.json")
		config.P2PPort = c.getOpenPort()
		config.Password = accountPassword
		config.LogLevel = logLevel
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
	var stdErr bytes.Buffer

	cmd := exec.Command(c.Config.Binary, args...) //nolint:gosec
	cmd.Stderr = &stdErr
	cmd.Stdout = c.Config.GetStdout(args[0])

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stdErr.String())
	}

	return nil
}

// EmitTransfer function is used to invoke e2e rootchain emit command
// with appropriately created wallets and amounts for test transactions
func (c *TestCluster) EmitTransfer(contractAddress, walletAddresses, amounts string) error {
	if len(contractAddress) == 0 {
		return errors.New("provide contractAddress value")
	}

	if len(walletAddresses) == 0 {
		return errors.New("provide at least one wallet address value")
	}

	if len(amounts) == 0 {
		return errors.New("provide at least one amount value")
	}

	return c.cmdRun("e2e",
		"rootchain",
		"emit",
		"--contract", contractAddress,
		"--wallets", walletAddresses,
		"--amounts", amounts)
}

func (c *TestCluster) Fail(err error) {
	c.once.Do(func() {
		c.executionErr = err
		close(c.failCh)
	})
}

func (c *TestCluster) Stop() {
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

		if !handler() {
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
			if srv.isRunning() && !fn(srv) { // if server is stopped - skip it
				return true
			}
		}

		return false
	})
}

func (c *TestCluster) getOpenPort() int64 {
	c.initialPort++

	return c.initialPort
}
