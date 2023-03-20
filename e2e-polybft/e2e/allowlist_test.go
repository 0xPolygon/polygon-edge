package e2e

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state/runtime/allowlist"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/wallet"
)

func TestAllowList_ContractDeployment(t *testing.T) {
	// create two accounts, one for an admin sender and a second
	// one for a non-enabled account that will switch on-off between
	// both enabled and non-enabled roles.
	admin, _ := wallet.GenerateKey()
	target, _ := wallet.GenerateKey()

	adminAddr := types.Address(admin.Address())
	targetAddr := types.Address(target.Address())

	otherAddr := types.Address{0x1}

	cluster := framework.NewTestCluster(t, 3,
		framework.WithPremine(adminAddr, targetAddr),
		framework.WithContractDeployerAllowListAdmin(adminAddr),
		framework.WithContractDeployerAllowListEnabled(otherAddr),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	// bytecode for an empty smart contract
	bytecode, _ := hex.DecodeString("6080604052348015600f57600080fd5b50603e80601d6000396000f3fe6080604052600080fdfea265627a7a7231582027748e4afe5ee282a786005d286f4427f13dac1b62e03f9aed311c2db7e8245364736f6c63430005110032")

	expectRole := func(addr types.Address, role allowlist.Role) {
		out := cluster.Call(t, contracts.AllowListContractsAddr, allowlist.ReadAllowListFunc, addr)

		num, ok := out["0"].(*big.Int)
		if !ok {
			t.Fatal("unexpected")
		}

		require.Equal(t, role.Uint64(), num.Uint64())
	}

	{
		// Step 0. Check the role of both accounts
		expectRole(adminAddr, allowlist.AdminRole)
		expectRole(targetAddr, allowlist.NoRole)
		expectRole(otherAddr, allowlist.EnabledRole)
	}

	{
		// Step 1. 'targetAddr' can send a normal transaction (non-contract creation).
		err := cluster.Transfer(t, target, types.ZeroAddress, big.NewInt(1)).Wait()
		require.NoError(t, err)
	}

	{
		// Step 2. 'targetAddr' **cannot** deploy a contract because it is not whitelisted.
		// (The transaction does not fail but the contract is not deployed and all gas
		// for the transaction is consumed)
		deployTxn := cluster.Deploy(t, target, bytecode)
		require.NoError(t, deployTxn.Wait())
		require.True(t, deployTxn.Reverted())
		require.False(t, cluster.ExistsCode(t, deployTxn.Receipt().ContractAddress))
	}

	{
		// Step 3. 'adminAddr' can create contracts
		deployTxn := cluster.Deploy(t, admin, bytecode)
		require.NoError(t, deployTxn.Wait())
		require.True(t, deployTxn.Succeed())
		require.True(t, cluster.ExistsCode(t, deployTxn.Receipt().ContractAddress))
	}

	{
		// Step 4. 'adminAddr' sends a transaction to enable 'targetAddr'.
		input, _ := allowlist.SetEnabledSignatureFunc.Encode([]interface{}{targetAddr})

		adminSetTxn := cluster.MethodTxn(t, admin, contracts.AllowListContractsAddr, input)
		require.NoError(t, adminSetTxn.Wait())
		expectRole(targetAddr, allowlist.EnabledRole)
	}

	{
		// Step 5. 'targetAddr' can create contracts now.
		deployTxn := cluster.Deploy(t, target, bytecode)
		require.NoError(t, deployTxn.Wait())
		require.True(t, deployTxn.Succeed())
		require.True(t, cluster.ExistsCode(t, deployTxn.Receipt().ContractAddress))
	}

	{
		// Step 6. 'targetAddr' cannot enable other accounts since it is not an admin
		// (The transaction fails)
		input, _ := allowlist.SetEnabledSignatureFunc.Encode([]interface{}{types.ZeroAddress})

		adminSetFailTxn := cluster.MethodTxn(t, target, contracts.AllowListContractsAddr, input)
		require.NoError(t, adminSetFailTxn.Wait())
		require.True(t, adminSetFailTxn.Failed())
		expectRole(types.ZeroAddress, allowlist.NoRole)
	}
}
