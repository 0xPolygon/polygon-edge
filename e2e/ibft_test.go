package e2e

import (
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
)

func TestIbft_Transfer(t *testing.T) {
	signer := &crypto.FrontierSigner{}
	senderKey, senderAddr := framework.GenerateKeyAndAddr(t)
	_, receiverAddr := framework.GenerateKeyAndAddr(t)

	// todo: same code
	dataDir, err := framework.TempDir()
	if err != nil {
		t.Fatal(err)
	}

	defaultCfg := func(i int, config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusIBFT)
		config.SetIBFTDirPrefix(IBFTDirPrefix)
		config.SetIBFTDir(fmt.Sprintf("%s%d", IBFTDirPrefix, i))
		config.Premine(senderAddr, framework.EthToWei(10))
		config.SetSeal(true)
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
	for _, srv := range srvs {
		if err := srv.WaitForReady(); err != nil {
			t.Fatal(err)
		}
	}
	//

	for i := 0; i < 3; i++ {
		txn := &types.Transaction{
			From:     senderAddr,
			To:       &receiverAddr,
			GasPrice: big.NewInt(10000),
			Gas:      1000000,
			Value:    framework.EthToWei(1),
			Nonce:    uint64(i),
		}
		txn, err = signer.SignTx(txn, senderKey)
		if err != nil {
			t.Fatal(err)
		}
		data := txn.MarshalRLP()

		hash, err := srvs[0].JSONRPC().Eth().SendRawTransaction(data)
		assert.NoError(t, err)
		assert.NotNil(t, hash)

		receipt, err := srvs[0].WaitForReceipt(hash)
		assert.NoError(t, err)
		assert.NotNil(t, receipt)
	}
}
