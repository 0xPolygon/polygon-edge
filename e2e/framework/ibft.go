package framework

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

type IBFTServersManager struct {
	t       *testing.T
	servers []*TestServer
}

type IBFTServerConfigCallback func(index int, config *TestServerConfig)

var startTime int64

func init() {
	startTime = time.Now().UTC().UnixMilli()
}

func NewIBFTServersManager(
	t *testing.T,
	numNodes int,
	ibftDirPrefix string,
	callback IBFTServerConfigCallback,
) *IBFTServersManager {
	t.Helper()

	dataDir, err := tempDir()
	if err != nil {
		t.Fatal(err)
	}

	srvs := make([]*TestServer, 0, numNodes)

	t.Cleanup(func() {
		for _, s := range srvs {
			s.Stop()
		}
		if err := os.RemoveAll(dataDir); err != nil {
			t.Log(err)
		}
	})

	bootnodes := make([]string, 0, numNodes)
	genesisValidators := make([]string, 0, numNodes)

	logsDir, err := initLogsDir(t)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < numNodes; i++ {
		srv := NewTestServer(t, dataDir, func(config *TestServerConfig) {
			config.SetConsensus(ConsensusIBFT)
			config.SetIBFTDirPrefix(ibftDirPrefix)
			config.SetIBFTDir(fmt.Sprintf("%s%d", ibftDirPrefix, i))
			config.SetLogsDir(logsDir)
			config.SetSaveLogs(true)
			config.SetName(fmt.Sprintf("node-%d", i))
			callback(i, config)
		})
		res, err := srv.SecretsInit()

		if err != nil {
			t.Fatal(err)
		}

		libp2pAddr := ToLocalIPv4LibP2pAddr(srv.Config.LibP2PPort, res.NodeID)

		srvs = append(srvs, srv)
		bootnodes = append(bootnodes, libp2pAddr)
		genesisValidators = append(genesisValidators, res.Address)
	}

	srv := srvs[0]
	srv.Config.SetBootnodes(bootnodes)
	// Set genesis staking balance for genesis validators
	for i, v := range genesisValidators {
		addr := types.StringToAddress(v)
		conf := srvs[i].Config

		if conf.GenesisValidatorBalance != nil {
			srv.Config.Premine(addr, conf.GenesisValidatorBalance)
		}
	}

	if err := srv.GenerateGenesis(); err != nil {
		t.Fatal(err)
	}

	if err := srv.GenesisPredeploy(); err != nil {
		t.Fatal(err)
	}

	return &IBFTServersManager{t, srvs}
}

func (m *IBFTServersManager) StartServers(ctx context.Context) {
	for idx, srv := range m.servers {
		if err := srv.Start(ctx); err != nil {
			m.t.Logf("server %d failed to start: %+v", idx, err)
			m.t.Fatal(err)
		}
	}

	for idx, srv := range m.servers {
		if err := srv.WaitForReady(ctx); err != nil {
			m.t.Logf("server %d couldn't advance block: %+v", idx, err)
			m.t.Fatal(err)
		}
	}
}

func (m *IBFTServersManager) StopServers() {
	for _, srv := range m.servers {
		srv.Stop()
	}
}

func (m *IBFTServersManager) GetServer(i int) *TestServer {
	if i >= len(m.servers) {
		return nil
	}

	return m.servers[i]
}

func initLogsDir(t *testing.T) (string, error) {
	t.Helper()
	logsDir := path.Join("..", fmt.Sprintf("e2e-logs-%d", startTime), t.Name())

	if err := common.CreateDirSafe(logsDir, 0755); err != nil {
		return "", err
	}

	return logsDir, nil
}
