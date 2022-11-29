package e2e

import (
	"context"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/contract"
	"github.com/umbracle/ethgo/wallet"
	"math/big"
	"testing"
	"time"
)

func TestExperiment(t *testing.T) {
	senderKey, _ := crypto.GenerateECDSAKey()
	sender := wallet.NewKey(senderKey)
	governanceKey, _ := crypto.GenerateECDSAKey()
	governance := wallet.NewKey(governanceKey)

	validatorSetSize := 5

	//epochReward := big.NewInt(1) //ethgo.Gwei(100)
	minStake := ethgo.Gwei(1)
	minDelegation := ethgo.Gwei(1)
	//wallet
	chainID := uint64(123)

	validatorsMap := map[types.Address]*big.Int{}

	var err error
	scPath := "../core-contracts/artifacts/contracts/"
	chainContracts := map[types.Address]*polybft.Artifact{}
	for _, v := range genesis.GenesisContracts {
		chainContracts[v.Address], err = polybft.ReadArtifact(scPath, v.RelativePath, v.Name)
		require.NoError(t, err)
	}

	cluster := framework.NewTestCluster(t, validatorSetSize,
		func(config *framework.TestClusterConfig) {
			config.WithLogs = true
		},
		framework.WithGenesisGenerator(func(validators []genesis.GenesisTarget) *chain.Chain {
			stakeMap := make(map[types.Address]*big.Int)
			for _, v := range validators {
				stakeMap[types.Address(v.Account.Ecdsa.Address())] = ethgo.Ether(100)
				validatorsMap[types.Address(v.Account.Ecdsa.Address())] = ethgo.Ether(100)
			}
			ch, err := genesis.NewTestPolyBFT(
				"test",
				chainID,
				2*time.Second,
				10,
				5,
				validators,
				types.Address(governance.Address()),
				stakeMap,
				scPath,
				nil,
				minStake,
				minDelegation,
			)
			require.NoError(t, err)
			//setup sender and governance balances
			ch.Genesis.Alloc[types.Address(sender.Address())] = &chain.GenesisAccount{
				Balance: ethgo.Ether(10),
			}
			ch.Genesis.Alloc[types.Address(governance.Address())] = &chain.GenesisAccount{
				Balance: ethgo.Ether(10),
			}
			return ch
		}),
	)
	defer cluster.Stop()

	client := cluster.Servers[0].JSONRPC().Eth()
	f := func(nonce uint64, receiver ethgo.Address, value *big.Int) ethgo.Hash {
		// estimate gas price
		gasPrice, err := client.GasPrice()
		assert.NoError(t, err)

		chainID, err := client.ChainID()
		assert.NoError(t, err)
		t.Log("chainID", chainID)

		// send transaction
		rawTxn := &ethgo.Transaction{
			From:     sender.Address(),
			To:       &receiver,
			GasPrice: gasPrice,
			Gas:      30000, // enough to send a transfer
			Value:    value,
			Nonce:    nonce,
		}

		signer := wallet.NewEIP155Signer(chainID.Uint64())
		signedTxn, err := signer.SignTx(rawTxn, sender)
		assert.NoError(t, err)

		txnRaw, err := signedTxn.MarshalRLPTo(nil)
		assert.NoError(t, err)

		hash, err := client.SendRawTransaction(txnRaw)
		assert.NoError(t, err)

		return hash
	}
	require.NoError(t, cluster.WaitForBlock(2, 1*time.Minute))
	h1 := f(1, ethgo.Address{}, big.NewInt(100))
	t.Log(client.GetTransactionByHash(h1))
	t.Log(client.GetTransactionReceipt(h1))

	go func() {
		ticker := time.Tick(time.Second * 20)
		for _ = range ticker {
			for i, s := range cluster.Servers {
				bl, err := s.JSONRPC().Eth().GetBlockByNumber(ethgo.Latest, false)
				if err != nil {
					t.Log(i, "get block", err)
					continue
				}
				t.Log(i, "block", bl.Number, bl.Hash, bl.StateRoot, bl.ParentHash)
			}
		}
	}()

	require.NoError(t, cluster.WaitForBlock(22, 2*time.Minute))
	t.Log("Validator's balances")
	for addr, balance := range validatorsMap {
		currentBalance, err := client.GetBalance(ethgo.Address(addr), ethgo.BlockNumber(21))
		require.NoError(t, err)
		t.Log(addr.String(), balance.Uint64(), currentBalance)
		t.Log()
	}

	validatorContract := contract.NewContract(
		ethgo.Address(contracts.ValidatorSetContract),
		chainContracts[contracts.ValidatorSetContract].Abi,
		contract.WithJsonRPC(client),
	)

	t.Log(validatorContract.Call("getCurrentValidatorSet", 21))
	t.Log(validatorContract.Call("currentEpochId", 2))
	t.Log(validatorContract.Call("currentEpochId", 11))
	t.Log(validatorContract.Call("currentEpochId", 21))
	t.Log("epochReward")
	t.Log(validatorContract.Call("epochReward", 21))
	for addr := range validatorsMap {
		res, err := validatorContract.Call("getValidator", 21, addr)
		require.NoError(t, err)
		t.Log(addr, res)
	}

	ctx, cancel := context.WithCancel(context.Background())

	var txNum uint64
	go func(ctx context.Context) {
		for {
			txNum++
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(time.Second * 5)
			}
			nonce, err := client.GetNonce(sender.Address(), ethgo.Latest)
			if err != nil {
				t.Log("get nonce err", err)
				continue
			}
			h := f(nonce, ethgo.Address{}, big.NewInt(100))
			ttt := time.Now()
			var receipt *ethgo.Receipt
			for {
				var err error

				receipt, err = client.GetTransactionReceipt(h)
				if err != nil {
					t.Log("get nonce err", err, "receipt", receipt)
					continue
				}
				if receipt != nil {
					t.Log("Receipt took", time.Since(ttt))
					break
				}
				time.Sleep(time.Second)
				//t.Log("receipt", h, "is nil")
			}

			if receipt.Status != uint64(types.ReceiptSuccess) {
				t.Log(txNum, "tx failed", receipt)
				continue
			}
			if txNum%50 == 0 {
				block, err := client.GetBlockByNumber(ethgo.Latest, false)
				if err != nil {
					t.Log(txNum, "get block failed", err)
				}
				t.Log("passed tx", txNum, "block", block.Number, " -------------------------------")
				t.Log(validatorContract.Call("getCurrentValidatorSet", ethgo.BlockNumber(block.Number)))
				t.Log(validatorContract.Call("currentEpochId", ethgo.BlockNumber(block.Number)))

				for addr := range validatorsMap {
					res, _ := validatorContract.Call("getValidator", 21, addr)
					//require.NoError(t, err)
					t.Log(addr, res)
				}

			}
		}
	}(ctx)

	require.NoError(t, cluster.WaitForBlock(13000, 1000*time.Minute))
	cancel()
}
