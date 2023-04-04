package e2e

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/wallet"
)

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
		deployTxn := cluster.Deploy(t, acct, contractsapi.TestSimple.Bytecode)
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
