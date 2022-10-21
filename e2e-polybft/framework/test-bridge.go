package framework

import (
	"bytes"
	"fmt"
	"os/exec"
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

	initContracts := []string{
		"rootchain",
		"init-contracts",
		"--path", t.clusterConfig.ContractsDir,
		"--validator-path", t.clusterConfig.TmpDir,
		"--validator-prefix", t.clusterConfig.ValidatorPrefix,
	}

	var stdErr bytes.Buffer

	cmd := exec.Command(resolveBinary(), initContracts...) //nolint:gosec
	cmd.Stderr = &stdErr
	cmd.Stdout = t.clusterConfig.GetStdout(args[0])

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stdErr.String())
	}

	return nil
}

func (t *TestBridge) Stop() {
	if err := t.node.Stop(); err != nil {
		t.t.Error(err)
	}

	t.node = nil
}
