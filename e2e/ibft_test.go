package e2e

import (
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
)

const (
	IbftDirPrefix    = "e2e-ibft-"
	IbftMinimumNodes = 4
)

func TestIbft_Transfer(t *testing.T) {
	var privKeyRaw = "0x4b2216c76f1b4c60c44d41986863e7337bc1a317d6a9366adfd8966fe2ac05f6"
	key, _ := crypto.ParsePrivateKey(hex.MustDecodeHex(privKeyRaw))
	addr := crypto.PubKeyToAddress(&key.PublicKey)

	dataDir, err := framework.TempDir()
	if err != nil {
		t.Fatal(err)
	}

	defaultCfg := func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusIBFT)
		config.SetIBFTDirPrefix(IbftDirPrefix)
		config.Premine(addr, framework.EthToWei(10))
		config.SetSeal(true)
	}

	srvs := make([]*framework.TestServer, 0, IbftMinimumNodes)
	bootnodes := make([]string, 0, IbftMinimumNodes)
	for i := 0; i < IbftMinimumNodes; i++ {
		srv := framework.NewTestServer(t, dataDir, defaultCfg)
		res, err := srv.InitIbft(i + 1)
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
	for i, srv := range srvs {
		if err := srv.Start(t, i+1); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(time.Second * 10)

	signer := crypto.NewEIP155Signer(100)
	target := types.StringToAddress("0x1010101010101010101010101010101010101010")

	for i := 0; i < 3; i++ {
		txn := &types.Transaction{
			From:     addr,
			To:       &target,
			GasPrice: big.NewInt(10000),
			Gas:      1000000,
			Value:    framework.EthToWei(1),
			Nonce:    uint64(i),
		}
		txn, err = signer.SignTx(txn, key)
		if err != nil {
			t.Fatal(err)
		}
		data := txn.MarshalRLP()

		hash, err := srvs[0].JSONRPC().Eth().SendRawTransaction(data)
		assert.NoError(t, err)
		assert.NotNil(t, hash)

		time.Sleep(time.Second * 5)

		receipt, err := srvs[0].JSONRPC().Eth().GetTransactionReceipt(hash)
		assert.NoError(t, err)
		assert.NotNil(t, receipt)
	}
}
