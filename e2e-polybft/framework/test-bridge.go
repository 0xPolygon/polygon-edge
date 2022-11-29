package framework

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"strconv"
	"testing"

	"github.com/0xPolygon/polygon-edge/command/rootchain/server"
)

type TestBridge struct {
	t             *testing.T
	clusterConfig *TestClusterConfig
	node          *node
}

func NewTestBridge(t *testing.T, clusterConfig *TestClusterConfig) (*TestBridge, error) {
	t.Helper()

	bridge := &TestBridge{
		t:             t,
		clusterConfig: clusterConfig,
	}

	err := bridge.Start()
	if err != nil {
		return nil, err
	}

	return bridge, nil
}

func (t *TestBridge) Start() error {
	// Build arguments
	args := []string{
		"rootchain",
		"server",
		"--data-dir", t.clusterConfig.Dir("test-rootchain"),
	}

	stdout := t.clusterConfig.GetStdout("bridge")

	bridgeNode, err := newNode(t.clusterConfig.Binary, args, stdout)
	if err != nil {
		return err
	}

	t.node = bridgeNode

	if err = server.PingServer(nil); err != nil {
		return err
	}

	return nil
}

func (t *TestBridge) deployRootchainContracts(genesisPath string) error {
	args := []string{
		"rootchain",
		"init-contracts",
		"--path", t.clusterConfig.ContractsDir,
		"--validator-path", t.clusterConfig.TmpDir,
		"--validator-prefix", t.clusterConfig.ValidatorPrefix,
		"--genesis-path", genesisPath,
	}

	err := runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("bridge"))
	if err != nil {
		return fmt.Errorf("failed to deploy rootchain contracts: %w", err)
	}

	return nil
}

func (t *TestBridge) fundValidators() error {
	args := []string{
		"rootchain",
		"fund",
		"--data-dir", path.Join(t.clusterConfig.TmpDir, t.clusterConfig.ValidatorPrefix),
		"--num", strconv.Itoa(int(t.clusterConfig.ValidatorSetSize) + t.clusterConfig.NonValidatorCount),
	}

	err := runCommand(t.clusterConfig.Binary, args, t.clusterConfig.GetStdout("bridge"))
	if err != nil {
		return fmt.Errorf("failed to deploy rootchain contracts: %w", err)
	}

	return nil
}

func (t *TestBridge) Stop() {
	if err := t.node.Stop(); err != nil {
		t.t.Error(err)
	}

	t.node = nil
}

// runCommand executes command with given arguments
func runCommand(binary string, args []string, stdout io.Writer) error {
	var stdErr bytes.Buffer

	cmd := exec.Command(binary, args...)
	cmd.Stderr = &stdErr
	cmd.Stdout = stdout

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stdErr.String())
	}

	if stdErr.Len() > 0 {
		return errors.New(stdErr.String())
	}

	return nil
}
