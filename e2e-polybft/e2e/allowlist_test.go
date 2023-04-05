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

// Contract used as bytecode

/*
pragma solidity ^0.5.5;
pragma experimental ABIEncoderV2;

contract Sample {
	address public constant allowList = 0x0200000000000000000000000000000000000000;

	function readAllowList(address addr) public returns (uint256) {
		(bool success, bytes memory returnData) = allowList.call(abi.encodeWithSignature("readAllowList(address)", addr));
		(uint256 val) = abi.decode(returnData, (uint256));
		return val;
	}
}
*/

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
	bytecode, err := hex.DecodeString("608060405234801561001057600080fd5b506103e4806100206000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c806387b9d25c1461003b578063eb54dae114610059575b600080fd5b610043610089565b60405161005091906102b8565b60405180910390f35b610073600480360361006e9190810190610200565b6100a1565b60405161008091906102d3565b60405180910390f35b73020000000000000000000000000000000000000081565b600080606073020000000000000000000000000000000000000073ffffffffffffffffffffffffffffffffffffffff16846040516024016100e291906102b8565b6040516020818303038152906040527feb54dae1000000000000000000000000000000000000000000000000000000007bffffffffffffffffffffffffffffffffffffffffffffffffffffffff19166020820180517bffffffffffffffffffffffffffffffffffffffffffffffffffffffff838183161783525050505060405161016c91906102a1565b6000604051808303816000865af19150503d80600081146101a9576040519150601f19603f3d011682016040523d82523d6000602084013e6101ae565b606091505b50915091506000818060200190516101c99190810190610229565b9050809350505050919050565b6000813590506101e581610373565b92915050565b6000815190506101fa8161038a565b92915050565b60006020828403121561021257600080fd5b6000610220848285016101d6565b91505092915050565b60006020828403121561023b57600080fd5b6000610249848285016101eb565b91505092915050565b61025b81610304565b82525050565b600061026c826102ee565b61027681856102f9565b9350610286818560208601610340565b80840191505092915050565b61029b81610336565b82525050565b60006102ad8284610261565b915081905092915050565b60006020820190506102cd6000830184610252565b92915050565b60006020820190506102e86000830184610292565b92915050565b600081519050919050565b600081905092915050565b600061030f82610316565b9050919050565b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b6000819050919050565b60005b8381101561035e578082015181840152602081019050610343565b8381111561036d576000848401525b50505050565b61037c81610304565b811461038757600080fd5b50565b61039381610336565b811461039e57600080fd5b5056fea365627a7a723158201e056518a3df30ba0dbbaf963d113a2cd9836f8d678c3d3a99a53a4fabe59ad26c6578706572696d656e74616cf564736f6c63430005110040")
	require.NoError(t, err)

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

	var proxyContract types.Address

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

		proxyContract = types.Address(deployTxn.Receipt().ContractAddress)
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

	{
		// Step 7. Deploy a proxy contract that can call the allow list
		resp := cluster.Call(t, proxyContract, allowlist.ReadAllowListFunc, adminAddr)

		role, ok := resp["0"].(*big.Int)
		require.True(t, ok)
		require.Equal(t, role.Uint64(), allowlist.AdminRole.Uint64())
	}
}

func TestAllowList_Transactions(t *testing.T) {
	// create two accounts, one for an admin sender and a second
	// one for a non-enabled account that will switch on-off between
	// both enabled and non-enabled roles.
	admin, _ := wallet.GenerateKey()
	target, _ := wallet.GenerateKey()
	other, _ := wallet.GenerateKey()

	adminAddr := types.Address(admin.Address())
	targetAddr := types.Address(target.Address())
	otherAddr := types.Address(other.Address())

	cluster := framework.NewTestCluster(t, 3,
		framework.WithPremine(adminAddr, targetAddr, otherAddr),
		framework.WithTransactionsAllowListAdmin(adminAddr),
		framework.WithTransactionsAllowListEnabled(otherAddr),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	// bytecode for an empty smart contract
	bytecode, _ := hex.DecodeString("6080604052348015600f57600080fd5b50603e80601d6000396000f3fe6080604052600080fdfea265627a7a7231582027748e4afe5ee282a786005d286f4427f13dac1b62e03f9aed311c2db7e8245364736f6c63430005110032")

	expectRole := func(addr types.Address, role allowlist.Role) {
		out := cluster.Call(t, contracts.AllowListTransactionsAddr, allowlist.ReadAllowListFunc, addr)

		num, ok := out["0"].(*big.Int)
		require.True(t, ok)
		require.Equal(t, role.Uint64(), num.Uint64())
	}

	{
		// Step 0. Check the role of both accounts
		expectRole(adminAddr, allowlist.AdminRole)
		expectRole(targetAddr, allowlist.NoRole)
		expectRole(otherAddr, allowlist.EnabledRole)
	}

	{
		// Step 1. 'otherAddr' can send a normal transaction (non-contract creation).
		otherTxn := cluster.Transfer(t, other, types.ZeroAddress, big.NewInt(1))
		require.NoError(t, otherTxn.Wait())
		require.True(t, otherTxn.Succeed())
	}

	{
		// Step 2. 'targetAddr' **cannot** send a normal transaction because it is not whitelisted.
		targetTxn := cluster.Transfer(t, target, types.ZeroAddress, big.NewInt(1))
		require.NoError(t, targetTxn.Wait())
		require.True(t, targetTxn.Reverted())
	}

	{
		// Step 2.5. 'targetAddr' **cannot** deploy a contract because it is not whitelisted.
		// (The transaction does not fail but the contract is not deployed and all gas
		// for the transaction is consumed)
		deployTxn := cluster.Deploy(t, target, bytecode)
		require.NoError(t, deployTxn.Wait())
		require.True(t, deployTxn.Reverted())
		require.False(t, cluster.ExistsCode(t, deployTxn.Receipt().ContractAddress))
	}

	{
		// Step 3. 'adminAddr' sends a transaction to enable 'targetAddr'.
		input, _ := allowlist.SetEnabledSignatureFunc.Encode([]interface{}{targetAddr})

		adminSetTxn := cluster.MethodTxn(t, admin, contracts.AllowListTransactionsAddr, input)
		require.NoError(t, adminSetTxn.Wait())
		expectRole(targetAddr, allowlist.EnabledRole)
	}

	{
		// Step 4. 'targetAddr' **can** send a normal transaction because it is whitelisted.
		targetTxn := cluster.Transfer(t, target, types.ZeroAddress, big.NewInt(1))
		require.NoError(t, targetTxn.Wait())
		require.True(t, targetTxn.Succeed())
	}

	{
		// Step 5. 'targetAddr' cannot enable other accounts since it is not an admin
		// (The transaction fails)
		input, _ := allowlist.SetEnabledSignatureFunc.Encode([]interface{}{types.ZeroAddress})

		adminSetFailTxn := cluster.MethodTxn(t, target, contracts.AllowListTransactionsAddr, input)
		require.NoError(t, adminSetFailTxn.Wait())
		require.True(t, adminSetFailTxn.Failed())
		expectRole(types.ZeroAddress, allowlist.NoRole)
	}
}
