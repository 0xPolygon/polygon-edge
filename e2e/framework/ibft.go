package framework

import (
	"context"
	"fmt"
	"testing"
)

type IBFTServersManager struct {
	t       *testing.T
	servers []*TestServer
}

type IBFTServerConfigCallback func(index int, config *TestServerConfig)

func NewIBFTServersManager(t *testing.T, numNodes int, rootDir string, ibftDirPrefix string, callback IBFTServerConfigCallback) *IBFTServersManager {
	t.Helper()

	srvs, bootnodes := make([]*TestServer, 0, numNodes), make([]string, 0, numNodes)
	t.Cleanup(func() {
		for _, s := range srvs {
			s.Stop()
		}
	})

	for i := 0; i < numNodes; i++ {
		srv := NewTestServer(t, rootDir, func(config *TestServerConfig) {
			config.SetConsensus(ConsensusIBFT)
			config.SetIBFTDirPrefix(ibftDirPrefix)
			config.SetIBFTDir(fmt.Sprintf("%s%d", ibftDirPrefix, i))
			callback(i, config)
		})
		res, err := srv.InitIBFT()
		if err != nil {
			t.Fatal(err)
		}
		libp2pAddr := ToLocalIPv4LibP2pAddr(srv.Config.LibP2PPort, res.NodeID)

		srvs = append(srvs, srv)
		bootnodes = append(bootnodes, libp2pAddr)
	}

	srvs[0].Config.SetBootnodes(bootnodes)
	if err := srvs[0].GenerateGenesis(); err != nil {
		t.Fatal(err)
	}

	return &IBFTServersManager{t, srvs}
}

func (m *IBFTServersManager) StartServers(ctx context.Context) {
	for _, srv := range m.servers {
		if err := srv.Start(ctx); err != nil {
			m.t.Fatal(err)
		}
	}
	for _, srv := range m.servers {
		if err := srv.WaitForReady(ctx); err != nil {
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
