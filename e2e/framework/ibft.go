package framework

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
)

type IBFTServersManager struct {
	t       *testing.T
	servers []*TestServer
}

type IBFTServerConfigCallback func(index int, config *TestServerConfig)

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

	for i := 0; i < numNodes; i++ {
		srv := NewTestServer(t, dataDir, func(config *TestServerConfig) {
			config.SetConsensus(ConsensusIBFT)
			config.SetIBFTDirPrefix(ibftDirPrefix)
			config.SetIBFTDir(fmt.Sprintf("%s%d", ibftDirPrefix, i))
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
