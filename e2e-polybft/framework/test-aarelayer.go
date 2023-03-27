package framework

import (
	"fmt"
	"io"
	"math/big"
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

// $ ./polygon-edge aarelayer sendtx --data-dir ./test-chain-2 --addr 127.0.0.1:8198
// --tx 0xc0ffee254729296a45a3885639AC7E10F9d54979:100:21000 --nonce 0
func (t *TestAARelayer) AASendTx(
	dataDir string,
	address types.Address,
	value *big.Int,
	gasLimit *big.Int,
	nonce uint64,
	waitForReceipt bool,
	stdout io.Writer,
) error {
	args := []string{
		"aarelayer", "sendtx",
		"--data-dir", dataDir,
		"--addr", t.config.Addr,
		"--tx", fmt.Sprintf("%s:%s:%s", address.String(), value.String(), gasLimit.String()),
		"--nonce", fmt.Sprintf("%d", nonce),
		"--invoker-addr", t.config.InvokerAddress.String(),
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
