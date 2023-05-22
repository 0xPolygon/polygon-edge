package framework

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/jsonrpc"
)

type RemoteCluster struct {
	chainConfigFilename string
	childRPCURL         *url.URL
	childRPCClient      *jsonrpc.Client
}

func NewRemoteCluster(t *testing.T, chainConfigFilename string, childRPCURL *url.URL) *RemoteCluster {
	t.Helper()

	client, err := jsonrpc.NewClient(childRPCURL.String())
	require.NoError(t, err)

	return &RemoteCluster{
		chainConfigFilename: chainConfigFilename,
		childRPCURL:         childRPCURL,
		childRPCClient:      client,
	}
}

func (rc *RemoteCluster) WaitForBlock(n uint64, timeout time.Duration) error {
	timer := time.NewTimer(timeout)

	for {
		// Wait and potentially timeout
		select {
		case <-timer.C:
			return fmt.Errorf("wait for block timeout")
		case <-time.After(2 * time.Second):
		}

		// Get latest block
		latestBlock, err := rc.JSONRPC().Eth().BlockNumber()
		if err != nil {
			return err
		}

		// Done
		if latestBlock >= n {
			return nil
		}
	}
}

func (rc *RemoteCluster) PolyBFTConfig(t *testing.T, chainConfigFileName string) polybft.PolyBFTConfig {
	t.Helper()

	polybftCfg, err := polybft.LoadPolyBFTConfig(rc.chainConfigFilename)
	require.NoError(t, err)

	return polybftCfg
}

func (rc *RemoteCluster) JSONRPCAddr() string {
	return rc.childRPCURL.String()
}

func (rc *RemoteCluster) JSONRPC() *jsonrpc.Client {
	return rc.childRPCClient
}
