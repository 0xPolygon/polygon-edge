package e2e

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/types"
)

func TestE2E_BurnContract_Deployed(t *testing.T) {
	contractKey, _ := wallet.GenerateKey()
	destinationKey, _ := wallet.GenerateKey()

	contractAddr := types.Address(contractKey.Address())
	destinationAddr := types.Address(destinationKey.Address())

	cluster := framework.NewTestCluster(t, 5,
		framework.WithBurnContract(fmt.Sprintf("0:%s:%s", contractAddr.String(), destinationAddr.String())),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)
	client := cluster.Servers[0].JSONRPC().Eth()

	// Get the code for the default deployed burn contract
	code, err := client.GetCode(ethgo.Address(contractAddr), ethgo.Latest)
	require.NoError(t, err)
	require.NotEqual(t, code, "0x")

	resp, err := client.GetStorageAt(ethgo.Address(contractAddr), ethgo.Hash{}, ethgo.Latest)
	require.NoError(t, err)
	require.NotEqual(t, "0x0000000000000000000000000000000000000000000000000000000000000000", resp.String())

}

// add the test when token is mintable and see that default cnt is not deployed
