package e2e

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
	"golang.org/x/crypto/sha3"

	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
)

func TestNewFilter_Logs(t *testing.T) {
	addrPrivKey, addr := tests.GenerateKeyAndAddr(t)
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.Premine(addr, framework.EthToWei(10))
		config.SetSeal(true)
	})
	srv := srvs[0]

	ctx1, cancel1 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel1()

	contractAddr, err := srv.DeployContract(ctx1, sampleByteCode, addrPrivKey)

	if err != nil {
		t.Fatal(err)
	}

	client := srv.JSONRPC()
	id, err := client.Eth().NewFilter(&web3.LogFilter{})
	assert.NoError(t, err)

	numCalls := 10
	for i := 0; i < numCalls; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.TxnTo(ctx, contractAddr, "setA1", addrPrivKey)
	}

	res, err := client.Eth().GetFilterChanges(id)
	assert.NoError(t, err)
	assert.Equal(t, len(res), numCalls)
}

func TestNewFilter_Block(t *testing.T) {
	signerKey, from := tests.GenerateKeyAndAddr(t)
	_, to := tests.GenerateKeyAndAddr(t)

	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.Premine(from, framework.EthToWei(10))
		config.SetSeal(true)
	})
	srv := srvs[0]
	client := srv.JSONRPC()
	signer := crypto.NewEIP155Signer(100)

	id, err := client.Eth().NewBlockFilter()
	assert.NoError(t, err)

	for i := 0; i < 3; i++ {
		signedTx, err := signer.SignTx(&types.Transaction{
			From:     srv.Config.PremineAccts[0].Addr,
			GasPrice: big.NewInt(10000),
			Gas:      1000000,
			To:       &to,
			Value:    big.NewInt(10000),
			Nonce:    uint64(i),
		}, signerKey)
		if err != nil {
			t.Error("Could not sign transacion")
		}

		hash, err := client.Eth().SendRawTransaction(signedTx.MarshalRLP())

		assert.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err = tests.WaitForReceipt(ctx, srv.JSONRPC().Eth(), hash)
		assert.NoError(t, err)
	}

	// there should be three changes
	blocks, err := client.Eth().GetFilterChangesBlock(id)
	assert.NoError(t, err)
	assert.Len(t, blocks, 3)
}

func TestFilterValue(t *testing.T) {
	// Scenario :
	//
	//	1.	Deploy a smart contract which is able to emit an event when calling a method.
	//		The event will contain a data, the number 42.
	//
	//		1a. Create a filter which will only register a specific event (
	//		MyEvent) emitted by the previously deployed contract.
	//
	//	2.	Call the smart contract method and wait for the block.
	//
	//	3.	Query the block's bloom filter to make sure the data has been properly inserted.
	//
	addrPrivKey, addr := tests.GenerateKeyAndAddr(t)
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.Premine(addr, framework.EthToWei(10))
		config.SetSeal(true)
	})
	srv := srvs[0]

	ctx1, cancel1 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel1()

	contractAddr, err := srv.DeployContract(ctx1, bloomFilterTestBytecode, addrPrivKey)

	if err != nil {
		t.Fatal(err)
	}

	client := srv.JSONRPC()

	// Encode event signature
	hash := sha3.NewLegacyKeccak256()
	decodeString := []byte("MyEvent(address,uint256)")
	hash.Write(decodeString)

	buf := hash.Sum(nil)

	// Convert to right format
	var placeholder web3.Hash

	copy(placeholder[:], buf)

	var filterEventHashes []*web3.Hash

	filterEventHashes = append(filterEventHashes, &placeholder)

	var filterAddresses []web3.Address

	filterAddresses = append(filterAddresses, contractAddr)

	id, err := client.Eth().NewFilter(&web3.LogFilter{
		Address: filterAddresses,
		Topics:  filterEventHashes,
	})
	assert.NoError(t, err)

	numCalls := 1
	for i := 0; i < numCalls; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.TxnTo(ctx, contractAddr, "TriggerMyEvent", addrPrivKey)
	}

	res, err := client.Eth().GetFilterChanges(id)
	assert.NoError(t, err)
	assert.Len(t, res, 1)
	assert.Equal(t, "0x000000000000000000000000000000000000000000000000000000000000002a", hex.EncodeToHex(res[0].Data))
}
