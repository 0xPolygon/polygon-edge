package framework

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
)

type TestAARelayerConfig struct {
	DataDir        string
	Addr           string
	JSONRPCAddress string
	Stdout         io.Writer
	DBPath         string
	Binary         string
	InvokerAddress types.Address
}

type TestAARelayer struct {
	t *testing.T

	config *TestAARelayerConfig
	node   *node

	sendTxNodes []*node
}

func NewTestAARelayer(t *testing.T, config *TestAARelayerConfig) *TestAARelayer {
	t.Helper()

	srv := &TestAARelayer{
		t:      t,
		config: config,
	}
	srv.Start()

	return srv
}

func (t *TestAARelayer) isRunning() bool {
	return t.node != nil
}

func (t *TestAARelayer) Start() {
	// Build arguments
	// $./polygon-edge aarelayer --addr 127.0.0.1:8198 --data-dir ./test-chain-1 --jsonrpc 127.0.0.1:10002
	args := []string{
		"aarelayer",
		"--addr", t.config.Addr,
		"--data-dir", t.config.DataDir,
		"--jsonrpc", t.config.JSONRPCAddress,
		"--db-path", t.config.DBPath,
		"--invoker-addr", t.config.InvokerAddress.String(),
		"--log-level", "DEBUG",
	}

	// Start the server
	node, err := newNode(t.config.Binary, args, t.config.Stdout)
	if err != nil {
		t.t.Fatal(err)
	}

	t.node = node
}

func (t *TestAARelayer) Stop() {
	if err := t.node.Stop(); err != nil {
		t.t.Fatal(err)
	}

	for _, n := range t.sendTxNodes {
		if err := n.Stop(); err != nil {
			t.t.Fatal(err)
		}
	}

	t.sendTxNodes = nil
	t.node = nil
}

// AASendTx executes aa sendtx command that sends aa transaction to the aa relayer
func (t *TestAARelayer) AASendTx(
	dataDir string,
	nonce uint64,
	payloadTxs []string, // slice of address:value:gasLimit
	waitForReceipt bool,
	stdout io.Writer,
) error {
	if len(payloadTxs) == 0 {
		return errors.New("there should be at least one transaction specified")
	}

	args := []string{
		"aarelayer", "sendtx",
		"--data-dir", dataDir,
		"--addr", t.config.Addr,
		"--nonce", fmt.Sprintf("%d", nonce),
		"--invoker-addr", t.config.InvokerAddress.String(),
	}

	for _, payload := range payloadTxs {
		args = append(args, "--tx", payload)
	}

	if !waitForReceipt {
		args = append(args, "--wait-for-receipt=false")
	}

	node, err := newNode(t.config.Binary, args, stdout)
	if err != nil {
		return err
	}

	t.sendTxNodes = append(t.sendTxNodes, node)

	return nil
}
