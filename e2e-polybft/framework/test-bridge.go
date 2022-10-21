package framework

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/0xPolygon/polygon-edge/command/rootchain/server"
)

type TestBridge struct {
	t             *testing.T
	clusterConfig *TestClusterConfig
	node          *node
	node2         *node
}

func NewTestBridge(t *testing.T, clusterConfig *TestClusterConfig) (*TestBridge, error) {
	bridge := &TestBridge{
		t:             t,
		clusterConfig: clusterConfig,
	}
	err := bridge.Start()
	return bridge, err
}

func (t *TestBridge) Start() error {

	fmt.Println("Start bridge")
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
	// ti := time.After(10 * time.Second)
	// go func() {
	//
	// 	select {
	// 	case <-ti:
	// 		fmt.Println("Starting init contracts")
	// 		args := []string{
	// 			"rootchain",
	// 			"init-contracts",
	// 		}
	// 		_, _ = newNode(t.clusterConfig.Binary, args, stdout)
	//
	// 	}
	//
	// }()

	initContracts := []string{
		"rootchain",
		"init-contracts",
	}
	cmd := exec.Command(resolveBinary(), initContracts...)
	cmd.Start()

	return server.PingServer(nil)
}

func (t *TestBridge) Stop() {
	if err := t.node.Stop(); err != nil {
		t.t.Error(err)
	}
	t.node = nil
}
