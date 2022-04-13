package e2e

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/abi"
)

const (
	//nolint:lll
	StateSyncerByteCode = "60806040526000805534801561001457600080fd5b5061042a806100246000396000f3fe608060405234801561001057600080fd5b506004361061002b5760003560e01c8063f928168914610030575b600080fd5b61004a60048036038101906100459190610139565b61004c565b005b8173ffffffffffffffffffffffffffffffffffffffff1660008081548092919061007590610311565b919050557f103fed9db65eac19c4d870f49ab7520fe03b99f1838e5996caf47e9e43308392836040516100a891906101ce565b60405180910390a35050565b60006100c76100c284610215565b6101f0565b9050828152602081018484840111156100e3576100e26103bd565b5b6100ee84828561029e565b509392505050565b600081359050610105816103dd565b92915050565b600082601f8301126101205761011f6103b8565b5b81356101308482602086016100b4565b91505092915050565b600080604083850312156101505761014f6103c7565b5b600061015e858286016100f6565b925050602083013567ffffffffffffffff81111561017f5761017e6103c2565b5b61018b8582860161010b565b9150509250929050565b60006101a082610246565b6101aa8185610251565b93506101ba8185602086016102ad565b6101c3816103cc565b840191505092915050565b600060208201905081810360008301526101e88184610195565b905092915050565b60006101fa61020b565b905061020682826102e0565b919050565b6000604051905090565b600067ffffffffffffffff8211156102305761022f610389565b5b610239826103cc565b9050602081019050919050565b600081519050919050565b600082825260208201905092915050565b600061026d82610274565b9050919050565b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b6000819050919050565b82818337600083830152505050565b60005b838110156102cb5780820151818401526020810190506102b0565b838111156102da576000848401525b50505050565b6102e9826103cc565b810181811067ffffffffffffffff8211171561030857610307610389565b5b80604052505050565b600061031c82610294565b91507fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff82141561034f5761034e61035a565b5b600182019050919050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b7f4e487b7100000000000000000000000000000000000000000000000000000000600052604160045260246000fd5b600080fd5b600080fd5b600080fd5b600080fd5b6000601f19601f8301169050919050565b6103e681610262565b81146103f157600080fd5b5056fea264697066735822122075ed654e1c6c272e01c886dd9e53d1a02e6b668c73e46754eee1e2341982a4f364736f6c63430008070033"
	//nolint:lll
	StateReceiverByteCode = "608060405234801561001057600080fd5b50610556806100206000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c80631865c57d1461003b578063ce425f8014610059575b600080fd5b610043610075565b6040516100509190610348565b60405180910390f35b610073600480360381019061006e919061026b565b610107565b005b6060600080546100849061043a565b80601f01602080910402602001604051908101604052809291908181526020018280546100b09061043a565b80156100fd5780601f106100d2576101008083540402835291602001916100fd565b820191906000526020600020905b8154815290600101906020018083116100e057829003601f168201915b5050505050905090565b806000908051906020019061011d929190610158565b507f4d6aef3909d2231ec4907ab3d8293de4b4b060601f237053051a76f2a505be788160405161014d9190610326565b60405180910390a150565b8280546101649061043a565b90600052602060002090601f01602090048101928261018657600085556101cd565b82601f1061019f57805160ff19168380011785556101cd565b828001600101855582156101cd579182015b828111156101cc5782518255916020019190600101906101b1565b5b5090506101da91906101de565b5090565b5b808211156101f75760008160009055506001016101df565b5090565b600061020e6102098461038f565b61036a565b90508281526020810184848401111561022a57610229610500565b5b6102358482856103f8565b509392505050565b600082601f830112610252576102516104fb565b5b81356102628482602086016101fb565b91505092915050565b6000602082840312156102815761028061050a565b5b600082013567ffffffffffffffff81111561029f5761029e610505565b5b6102ab8482850161023d565b91505092915050565b60006102bf826103c0565b6102c981856103d6565b93506102d9818560208601610407565b6102e28161050f565b840191505092915050565b60006102f8826103cb565b61030281856103e7565b9350610312818560208601610407565b61031b8161050f565b840191505092915050565b6000602082019050818103600083015261034081846102b4565b905092915050565b6000602082019050818103600083015261036281846102ed565b905092915050565b6000610374610385565b9050610380828261046c565b919050565b6000604051905090565b600067ffffffffffffffff8211156103aa576103a96104cc565b5b6103b38261050f565b9050602081019050919050565b600081519050919050565b600081519050919050565b600082825260208201905092915050565b600082825260208201905092915050565b82818337600083830152505050565b60005b8381101561042557808201518184015260208101905061040a565b83811115610434576000848401525b50505050565b6000600282049050600182168061045257607f821691505b602082108114156104665761046561049d565b5b50919050565b6104758261050f565b810181811067ffffffffffffffff82111715610494576104936104cc565b5b80604052505050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052602260045260246000fd5b7f4e487b7100000000000000000000000000000000000000000000000000000000600052604160045260246000fd5b600080fd5b600080fd5b600080fd5b600080fd5b6000601f19601f830116905091905056fea26469706673582212208cd49491d52ffea220213fd6003ee5ba83b2ad49bb3b117bc911f00908fe1d0a64736f6c63430008070033"

	StateSyncerABIJSON = `[
    {
      "anonymous": false,
      "inputs": [
        {
          "indexed": true,
          "internalType": "uint256",
          "name": "id",
          "type": "uint256"
        },
        {
          "indexed": true,
          "internalType": "address",
          "name": "contractAddress",
          "type": "address"
        },
        {
          "indexed": false,
          "internalType": "bytes",
          "name": "data",
          "type": "bytes"
        }
      ],
      "name": "StateSynced",
      "type": "event"
    },
    {
      "inputs": [
        {
          "internalType": "address",
          "name": "contractAddress",
          "type": "address"
        },
        {
          "internalType": "bytes",
          "name": "data",
          "type": "bytes"
        }
      ],
      "name": "stateSync",
      "outputs": [],
      "stateMutability": "nonpayable",
      "type": "function"
    }
  ]`

	StateReceiverABIJSON = `[
    {
      "anonymous": false,
      "inputs": [
        {
          "indexed": false,
          "internalType": "bytes",
          "name": "data",
          "type": "bytes"
        }
      ],
      "name": "StateReceived",
      "type": "event"
    },
    {
      "inputs": [],
      "name": "getState",
      "outputs": [
        {
          "internalType": "string",
          "name": "",
          "type": "string"
        }
      ],
      "stateMutability": "view",
      "type": "function"
    },
    {
      "inputs": [
        {
          "internalType": "bytes",
          "name": "data",
          "type": "bytes"
        }
      ],
      "name": "onStateReceived",
      "outputs": [],
      "stateMutability": "nonpayable",
      "type": "function"
    }
  ]`
)

var (
	StateSyncerABI   = abi.MustNewABI(StateSyncerABIJSON)
	StateRecieverABI = abi.MustNewABI(StateReceiverABIJSON)
)

func TestBridge_StateSync(t *testing.T) {
	senderKey, senderAddr := tests.GenerateKeyAndAddr(t)

	// Start root chain
	sourceIBFT := framework.NewIBFTServersManager(
		t,
		IBFTMinNodes,
		fmt.Sprintf("%ssource", IBFTDirPrefix),
		func(i int, config *framework.TestServerConfig) {
			config.Premine(senderAddr, framework.EthToWei(10))
			config.SetSeal(true)
			config.SetShowsLog(i == 0)
		})

	startSourceIBFTCtx, startSourceIBFTCancel := context.WithTimeout(context.Background(), time.Minute)
	defer startSourceIBFTCancel()
	sourceIBFT.StartServers(startSourceIBFTCtx)

	// deploy state syncer contract
	deployStateSyncerCtx, deployStateSyncerCancel := context.WithTimeout(context.Background(), time.Minute)
	defer deployStateSyncerCancel()

	syncerContractAddr, err := sourceIBFT.GetServer(0).DeployContract(deployStateSyncerCtx, StateSyncerByteCode, senderKey)

	if err != nil {
		t.Fatal(err)
	}

	// setup destination chain
	destIBFT := framework.NewIBFTServersManager(
		t,
		IBFTMinNodes,
		fmt.Sprintf("%sdest", IBFTDirPrefix),
		func(i int, config *framework.TestServerConfig) {
			config.SetSeal(true)
			config.Premine(senderAddr, framework.EthToWei(10))
			config.SetUseBridge(true)
			config.SetBridgeRootChainURL(sourceIBFT.GetServer(0).WSJSONRPCURL())
			config.SetBridgeRootChainContract(syncerContractAddr.String())
			config.SetBridgeRootChainConfirmations(5)
		})

	startDestIBFTCtx, startDestIBFTCancel := context.WithTimeout(context.Background(), time.Minute)
	defer startDestIBFTCancel()
	destIBFT.StartServers(startDestIBFTCtx)

	// deploy state receiver contract
	deployStateReceiverCtx, deployStateReceiverCancel := context.WithTimeout(
		context.Background(),
		time.Minute,
	)
	defer deployStateReceiverCancel()

	reciverContractAddr, err := destIBFT.GetServer(0).DeployContract(
		deployStateReceiverCtx,
		StateReceiverByteCode,
		senderKey,
	)

	if err != nil {
		t.Fatal(err)
	}

	// testSync is a helper function to bridge state between 2 chains
	testSync := func(t *testing.T, syncData string) {
		t.Helper()

		// Send state sync contract
		to := types.BytesToAddress(syncerContractAddr[:])
		input, err := StateSyncerABI.Methods["stateSync"].Encode(map[string]interface{}{
			"contractAddress": reciverContractAddr.String(),
			"data":            []byte(syncData),
		})

		if err != nil {
			t.Fatal(err)
		}

		sendStateSyncCtx, sendStateSyncCancel := context.WithTimeout(context.Background(), time.Minute)
		defer sendStateSyncCancel()

		_, err = sourceIBFT.GetServer(0).SendRawTx(sendStateSyncCtx, &framework.PreparedTransaction{
			Gas:      framework.DefaultGasLimit,
			GasPrice: big.NewInt(framework.DefaultGasPrice),
			To:       &to,
			From:     senderAddr,
			Input:    input,
		}, senderKey)
		if err != nil {
			t.Fatal(err)
		}

		err = waitUntilStateReachesToContract(
			context.Background(),
			destIBFT.GetServer(0),
			senderAddr,
			reciverContractAddr,
			syncData,
		)
		assert.NoError(t, err)
	}

	testSync(t, "hello")
	testSync(t, "hello, world")
	testSync(t, "hello") // duplicate data
}

// waitUntilStateReachesToContract repeats to get state in StateReceiver contract until expected state is set
func waitUntilStateReachesToContract(
	ctx context.Context,
	srv *framework.TestServer,
	senderAddress types.Address,
	contractAddress web3.Address,
	expectedState string,
) error {
	_, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		state, err := getStateReceiverState(
			srv,
			senderAddress,
			contractAddress,
		)
		if err != nil {
			return err, true
		}

		if state != expectedState {
			return nil, true
		}

		return nil, false
	})

	return err
}

func getStateReceiverState(
	srv *framework.TestServer,
	senderAddress types.Address,
	contractAddress web3.Address,
) (string, error) {
	response, err := srv.JSONRPC().Eth().Call(
		&web3.CallMsg{
			From:     web3.Address(senderAddress),
			To:       &contractAddress,
			Data:     StateRecieverABI.Methods["getState"].ID(),
			GasPrice: 100000000,
			Value:    big.NewInt(0),
		},
		web3.Latest,
	)
	if err != nil {
		return "", err
	}

	byteData, err := types.ParseBytes(&response)
	if err != nil {
		return "", err
	}

	res, err := StateRecieverABI.Methods["getState"].Outputs.Decode(byteData)
	if err != nil {
		return "", err
	}

	resMap, ok := res.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("expected map but got %T", resMap)
	}

	state, ok := resMap["0"].(string)
	if !ok {
		return "", fmt.Errorf("expected string in first returns but got %T", resMap["0"])
	}

	return state, nil
}
