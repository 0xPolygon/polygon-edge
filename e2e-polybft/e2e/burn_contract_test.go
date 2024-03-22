package e2e

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

func TestE2E_BurnContract_Deployed(t *testing.T) {
	contractKey, _ := crypto.GenerateECDSAKey()
	destinationKey, _ := crypto.GenerateECDSAKey()

	contractAddr := types.Address(contractKey.Address())
	destinationAddr := types.Address(destinationKey.Address())

	cluster := framework.NewTestCluster(t, 5,
		framework.WithBridge(),
		framework.WithNativeTokenConfig(nativeTokenNonMintableConfig),
		framework.WithTestRewardToken(),
		framework.WithBurnContract(&polybft.BurnContractInfo{
			Address:            contractAddr,
			DestinationAddress: destinationAddr,
		}),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)
	client := cluster.Servers[0].JSONRPC().Eth()

	// Get the code for the default deployed burn contract
	code, err := client.GetCode(ethgo.Address(contractAddr), ethgo.Latest)
	require.NoError(t, err)
	require.NotEqual(t, code, "0x")
}
