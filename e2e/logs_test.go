package e2e

import (
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
)

func TestNewFilter_Logs(t *testing.T) {
	_, addr := framework.GenerateKeyAndAddr(t)

	// todo: same code
	dataDir, err := framework.TempDir()
	if err != nil {
		t.Fatal(err)
	}

	defaultCfg := func(index int, config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusIBFT)
		config.SetIBFTDirPrefix(IBFTDirPrefix)
		config.Premine(addr, framework.EthToWei(10))
		config.SetSeal(true)
		config.SetDev(true)
		config.SetShowsLog(index == 0)
	}

	srvs := make([]*framework.TestServer, 0, IBFTMinNodes)
	bootnodes := make([]string, 0, IBFTMinNodes)
	for i := 0; i < IBFTMinNodes; i++ {
		srv := framework.NewTestServer(t, dataDir, func(config *framework.TestServerConfig) {
			defaultCfg(i, config)
		})
		res, err := srv.InitIBFT()
		if err != nil {
			t.Fatal(err)
		}
		libp2pAddr := framework.ToLocalIPv4LibP2pAddr(srv.Config.LibP2PPort, res.NodeID)

		srvs = append(srvs, srv)
		bootnodes = append(bootnodes, libp2pAddr)
	}
	t.Cleanup(func() {
		for _, srv := range srvs {
			srv.Stop()
		}
		if err := os.RemoveAll(dataDir); err != nil {
			t.Log(err)
		}
	})

	// Generate genesis.json
	srvs[0].Config.SetBootnodes(bootnodes)
	if err := srvs[0].GenerateGenesis(); err != nil {
		t.Fatal(err)
	}
	for _, srv := range srvs {
		if err := srv.Start(); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(time.Second * 5)

	contractAddr, err := srvs[0].DeployContract(sampleByteCode)
	if err != nil {
		t.Fatal(err)
	}

	client := srvs[0].JSONRPC()
	id, err := client.Eth().NewFilter(&web3.LogFilter{})
	assert.NoError(t, err)

	for i := 0; i < 10; i++ {
		srvs[0].TxnTo(contractAddr, "setA1")
	}

	res, err := client.Eth().GetFilterChanges(id)
	assert.NoError(t, err)
	assert.Len(t, res, 10)
}

func TestNewFilter_Block(t *testing.T) {
	_, from := framework.GenerateKeyAndAddr(t)
	target := web3.HexToAddress("0x1010101010101010101010101010101010101010")

	//todo: same code
	dataDir, err := framework.TempDir()
	if err != nil {
		t.Fatal(err)
	}

	defaultCfg := func(index int, config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusIBFT)
		config.SetIBFTDirPrefix(IBFTDirPrefix)
		config.Premine(from, framework.EthToWei(10))
		config.SetSeal(true)
		config.SetDev(true)
		config.SetShowsLog(index == 0)
	}

	srvs := make([]*framework.TestServer, 0, IBFTMinNodes)
	bootnodes := make([]string, 0, IBFTMinNodes)
	for i := 0; i < IBFTMinNodes; i++ {
		srv := framework.NewTestServer(t, dataDir, func(config *framework.TestServerConfig) {
			defaultCfg(i, config)
		})
		res, err := srv.InitIBFT()
		if err != nil {
			t.Fatal(err)
		}
		libp2pAddr := framework.ToLocalIPv4LibP2pAddr(srv.Config.LibP2PPort, res.NodeID)

		srvs = append(srvs, srv)
		bootnodes = append(bootnodes, libp2pAddr)
	}
	t.Cleanup(func() {
		for _, srv := range srvs {
			srv.Stop()
		}
		if err := os.RemoveAll(dataDir); err != nil {
			t.Log(err)
		}
	})

	// Generate genesis.json
	srvs[0].Config.SetBootnodes(bootnodes)
	if err := srvs[0].GenerateGenesis(); err != nil {
		t.Fatal(err)
	}
	for _, srv := range srvs {
		if err := srv.Start(); err != nil {
			t.Fatal(err)
		}
	}
	//

	client := srvs[0].JSONRPC()
	id, err := client.Eth().NewBlockFilter()
	assert.NoError(t, err)

	for i := 0; i < 3; i++ {
		_, err := client.Eth().SendTransaction(&web3.Transaction{
			From:     web3.HexToAddress(srvs[0].Config.PremineAccts[0].Addr.String()),
			To:       &target,
			GasPrice: 10000,
			Gas:      1000000,
			Value:    big.NewInt(10000),
			Nonce:    uint64(i),
		})
		assert.NoError(t, err)
	}
	time.Sleep(5 * time.Second)

	// there should be three changes
	blocks, err := client.Eth().GetFilterChangesBlock(id)
	assert.NoError(t, err)
	fmt.Println(blocks)
}
