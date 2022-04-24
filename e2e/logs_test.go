package e2e

import (
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/types"

	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
	"golang.org/x/crypto/sha3"
)

func TestNewFilter_Logs(t *testing.T) {
	key, addr := tests.GenerateKeyAndAddr(t)

	ibftManager := framework.NewIBFTServersManager(
		t,
		1,
		IBFTDirPrefix,
		func(i int, config *framework.TestServerConfig) {
			config.Premine(addr, framework.EthToWei(10))
			config.SetBlockTime(1)
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	ibftManager.StartServers(ctx)
	srv := ibftManager.GetServer(0)

	ctx1, cancel1 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel1()

	contractAddr, err := srv.DeployContract(ctx1, sampleByteCode, key)
	castedContractAddr := types.Address(contractAddr)

	if err != nil {
		t.Fatal(err)
	}

	txpoolClient := srv.TxnPoolOperator()
	jsonRPCClient := srv.JSONRPC()
	id, err := jsonRPCClient.Eth().NewFilter(&web3.LogFilter{})
	assert.NoError(t, err)

	numCalls := 10

	var (
		wg sync.WaitGroup
	)

	for i := 1; i <= numCalls; i++ {
		wg.Add(1)

		go func(nonce uint64) {
			defer wg.Done()

			txn, err := tests.GenerateAddTxnReq(tests.GenerateTxReqParams{
				Nonce:         nonce,
				ReferenceAddr: addr,
				ReferenceKey:  key,
				ToAddress:     castedContractAddr,
				GasPrice:      big.NewInt(framework.DefaultGasPrice),
				Input:         framework.MethodSig("setA1"),
			})
			if err != nil {
				return
			}

			addTxnContext, cancelFn := context.WithTimeout(context.Background(), time.Second*10)
			defer cancelFn()

			addResp, addErr := txpoolClient.AddTxn(addTxnContext, txn)
			if addErr != nil {
				return
			}

			receiptContext, cancelFn := context.WithTimeout(context.Background(), time.Second*10)
			defer cancelFn()

			txHash := web3.Hash(types.StringToHash(addResp.TxHash))
			if _, receiptErr := srv.WaitForReceipt(receiptContext, txHash); receiptErr != nil {
				return
			}
		}(uint64(i))
	}

	wg.Wait()

	res, err := jsonRPCClient.Eth().GetFilterChanges(id)

	assert.NoError(t, err)
	assert.Equal(t, numCalls, len(res))
}

func TestNewFilter_Block(t *testing.T) {
	fromKey, from := tests.GenerateKeyAndAddr(t)
	_, to := tests.GenerateKeyAndAddr(t)

	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.Premine(from, framework.EthToWei(10))
		config.SetSeal(true)
	})
	srv := srvs[0]
	client := srv.JSONRPC()

	id, err := client.Eth().NewBlockFilter()
	assert.NoError(t, err)

	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := srv.SendRawTx(ctx, &framework.PreparedTransaction{
			From:     from,
			To:       &to,
			GasPrice: big.NewInt(10000),
			Gas:      1000000,
			Value:    big.NewInt(10000),
		}, fromKey)

		assert.NoError(t, err)

		cancel()
	}

	// verify filter picked up block changes
	blocks, err := client.Eth().GetFilterChangesBlock(id)
	assert.NoError(t, err)
	assert.Greater(t, len(blocks), 0)
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
	key, addr := tests.GenerateKeyAndAddr(t)
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.Premine(addr, framework.EthToWei(10))
		config.SetSeal(true)
	})
	srv := srvs[0]

	ctx1, cancel1 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel1()

	contractAddr, err := srv.DeployContract(ctx1, bloomFilterTestBytecode, key)

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
	var (
		placeholderWrapper []*web3.Hash
		placeholder        web3.Hash
	)

	copy(placeholder[:], buf)
	placeholderWrapper = append(placeholderWrapper, &placeholder)

	var filterEventHashes [][]*web3.Hash

	filterEventHashes = append(filterEventHashes, placeholderWrapper)

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
		srv.InvokeMethod(ctx, types.Address(contractAddr), "TriggerMyEvent", key)
	}

	res, err := client.Eth().GetFilterChanges(id)
	assert.NoError(t, err)
	assert.Len(t, res, 1)
	assert.Equal(t, "0x000000000000000000000000000000000000000000000000000000000000002a", hex.EncodeToHex(res[0].Data))
}
