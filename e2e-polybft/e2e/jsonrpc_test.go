package e2e

import (
	"encoding/hex"
	"testing"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/wallet"
)

/*
contract Sample {
	uint256 val;

	function getValue() public returns (uint256) {
		return val;
	}
}
*/

var simpleCallContract = "608060405234801561001057600080fd5b5060c38061001f6000396000f3fe6080604052348015600f57600080fd5b506004361060285760003560e01c80632096525514602d575b600080fd5b60336047565b604051603e9190605d565b60405180910390f35b60008054905090565b6057816076565b82525050565b6000602082019050607060008301846050565b92915050565b600081905091905056fea365627a7a7231582071227b651950047ffed0fb11e7717a1fab7eb241a8944de583d00722050112596c6578706572696d656e74616cf564736f6c63430005110040"

func TestE2E_JsonRPC(t *testing.T) {
	acct, _ := wallet.GenerateKey()

	cluster := framework.NewTestCluster(t, 3,
		framework.WithPremine(types.Address(acct.Address())),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	client := cluster.Servers[0].JSONRPC().Eth()

	// Test eth_call with override in state diff
	t.Run("eth_call state override", func(t *testing.T) {
		bytecode, _ := hex.DecodeString(simpleCallContract)

		deployTxn := cluster.Deploy(t, acct, bytecode)
		require.NoError(t, deployTxn.Wait())
		require.True(t, deployTxn.Succeed())

		target := deployTxn.Receipt().ContractAddress

		input := abi.MustNewMethod("function getValue() public returns (uint256)").ID()

		resp, err := client.Call(&ethgo.CallMsg{To: &target, Data: input}, ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, "0x0000000000000000000000000000000000000000000000000000000000000000", resp)

		override := &ethgo.StateOverride{
			target: ethgo.OverrideAccount{
				StateDiff: &map[ethgo.Hash]ethgo.Hash{
					// storage slot 0 stores the 'val' uint256 value
					{0x0}: {0x3},
				},
			},
		}

		resp, err = client.Call(&ethgo.CallMsg{To: &target, Data: input}, ethgo.Latest, override)
		require.NoError(t, err)

		require.Equal(t, "0x0300000000000000000000000000000000000000000000000000000000000000", resp)
	})
}
