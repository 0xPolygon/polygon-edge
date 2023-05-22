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
	blockTime           time.Duration
}

func NewRemoteCluster(t *testing.T, chainConfigFilename string,
	childRPCURL *url.URL, blockTime time.Duration) *RemoteCluster {
	t.Helper()

	client, err := jsonrpc.NewClient(childRPCURL.String())
	require.NoError(t, err)

	return &RemoteCluster{
		chainConfigFilename: chainConfigFilename,
		childRPCURL:         childRPCURL,
		childRPCClient:      client,
		blockTime:           blockTime,
	}
}

func (rc *RemoteCluster) WaitForBlock(t *testing.T, n uint64, timeout time.Duration) {
	t.Helper()

	expectedDuration := timeout

	if n > 0 {
		latestBlockNum, err := rc.childRPCClient.Eth().BlockNumber()
		require.NoError(t, err)

		expectedDuration = time.Duration(int64(n-latestBlockNum) * int64(rc.blockTime))
	}

	timer := time.NewTimer(expectedDuration)
	t.Log("waiting", expectedDuration.String())

	for {
		// Wait and potentially timeout
		select {
		case <-timer.C:
			require.NoError(t, fmt.Errorf("wait for block timeout"))

			return
		case <-time.After(rc.blockTime):
		}

		// Get latest block
		latestBlock, err := rc.JSONRPC().Eth().BlockNumber()
		require.NoError(t, err)

		t.Log("block:", latestBlock)

		// Done
		if latestBlock >= n {
			return
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
