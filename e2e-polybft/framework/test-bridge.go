package framework

import (
	"os/exec"
	"testing"
	"time"

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

	// TODO: read stdout until some string is written into it (for example `Http server started` line)
	time.Sleep(time.Second * 10)

	initContracts := []string{
		"rootchain",
		"init-contracts",
	}

	cmd := exec.Command(resolveBinary(), initContracts...) //nolint:gosec
	if err := cmd.Run(); err != nil {
		return err
	}

	return server.PingServer(nil)
}

func (t *TestBridge) Stop() {
	if err := t.node.Stop(); err != nil {
		t.t.Error(err)
	}

	t.node = nil
}
