package e2e

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state/runtime/addresslist"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/wallet"
)

// Contract used as bytecode

/*
pragma solidity ^0.8;
pragma experimental ABIEncoderV2;

contract Sample {
	address public constant allowListContractsAddr = 0x0200000000000000000000000000000000000000;

	function readAddressList(address addr) public returns (uint256) {
		(bool success, bytes memory returnData) = allowListContractsAddr.call(abi.encodeWithSignature("readAddressList(address)", addr));
		(uint256 val) = abi.decode(returnData, (uint256));
		return val;
	}
}
*/

func TestE2E_AllowList_ContractDeployment(t *testing.T) {
	// create two accounts, one for an admin sender and a second
	// one for a non-enabled account that will switch on-off between
	// both enabled and non-enabled roles.
	admin, _ := wallet.GenerateKey()
	target, _ := wallet.GenerateKey()

	adminAddr := types.Address(admin.Address())
	targetAddr := types.Address(target.Address())

	otherAddr := types.Address{0x1}

	cluster := framework.NewTestCluster(t, 5,
		framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, adminAddr)),
		framework.WithPremine(adminAddr, targetAddr),
		framework.WithContractDeployerAllowListAdmin(adminAddr),
		framework.WithContractDeployerAllowListEnabled(otherAddr),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	// bytecode for an empty smart contract
	bytecode, err := hex.DecodeString("608060405234801561001057600080fd5b506103db806100206000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c80635e9f13f81461003b578063d78bca6914610059575b600080fd5b610043610089565b6040516100509190610217565b60405180910390f35b610073600480360381019061006e9190610263565b6100a1565b60405161008091906102a9565b60405180910390f35b73020000000000000000000000000000000000000081565b600080600073020000000000000000000000000000000000000073ffffffffffffffffffffffffffffffffffffffff16846040516024016100e29190610217565b6040516020818303038152906040527fd78bca69000000000000000000000000000000000000000000000000000000007bffffffffffffffffffffffffffffffffffffffffffffffffffffffff19166020820180517bffffffffffffffffffffffffffffffffffffffffffffffffffffffff838183161783525050505060405161016c9190610335565b6000604051808303816000865af19150503d80600081146101a9576040519150601f19603f3d011682016040523d82523d6000602084013e6101ae565b606091505b50915091506000818060200190518101906101c99190610378565b9050809350505050919050565b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b6000610201826101d6565b9050919050565b610211816101f6565b82525050565b600060208201905061022c6000830184610208565b92915050565b600080fd5b610240816101f6565b811461024b57600080fd5b50565b60008135905061025d81610237565b92915050565b60006020828403121561027957610278610232565b5b60006102878482850161024e565b91505092915050565b6000819050919050565b6102a381610290565b82525050565b60006020820190506102be600083018461029a565b92915050565b600081519050919050565b600081905092915050565b60005b838110156102f85780820151818401526020810190506102dd565b60008484015250505050565b600061030f826102c4565b61031981856102cf565b93506103298185602086016102da565b80840191505092915050565b60006103418284610304565b915081905092915050565b61035581610290565b811461036057600080fd5b50565b6000815190506103728161034c565b92915050565b60006020828403121561038e5761038d610232565b5b600061039c84828501610363565b9150509291505056fea264697066735822122035f391b5c3dbf9a5e31072167a4ada71e9ba762650849f6320bc6150fa45aa9564736f6c63430008110033")
	require.NoError(t, err)

	{
		// Step 0. Check the role of both accounts
		expectRole(t, cluster, contracts.AllowListContractsAddr, adminAddr, addresslist.AdminRole)
		expectRole(t, cluster, contracts.AllowListContractsAddr, targetAddr, addresslist.NoRole)
		expectRole(t, cluster, contracts.AllowListContractsAddr, otherAddr, addresslist.EnabledRole)
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
		input, _ := addresslist.SetEnabledFunc.Encode([]interface{}{targetAddr})

		adminSetTxn := cluster.MethodTxn(t, admin, contracts.AllowListContractsAddr, input)
		require.NoError(t, adminSetTxn.Wait())
		expectRole(t, cluster, contracts.AllowListContractsAddr, targetAddr, addresslist.EnabledRole)
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
		input, _ := addresslist.SetEnabledFunc.Encode([]interface{}{types.ZeroAddress})

		adminSetFailTxn := cluster.MethodTxn(t, target, contracts.AllowListContractsAddr, input)
		require.NoError(t, adminSetFailTxn.Wait())
		require.True(t, adminSetFailTxn.Failed())
		expectRole(t, cluster, contracts.AllowListContractsAddr, types.ZeroAddress, addresslist.NoRole)
	}

	{
		// Step 7. Call a proxy contract that can call the allow list
		resp := cluster.Call(t, proxyContract, addresslist.ReadAddressListFunc, adminAddr)

		role, ok := resp["0"].(*big.Int)
		require.True(t, ok)
		require.Equal(t, role.Uint64(), addresslist.AdminRole.Uint64())
	}
}

func TestE2E_BlockList_ContractDeployment(t *testing.T) {
	// create two accounts, one for an admin sender and a second
	// one for a non-enabled account that will switch on-off between
	// both enabled and non-enabled roles.
	admin, _ := wallet.GenerateKey()
	target, _ := wallet.GenerateKey()

	adminAddr := types.Address(admin.Address())
	targetAddr := types.Address(target.Address())

	otherAddr := types.Address{0x1}

	cluster := framework.NewTestCluster(t, 5,
		framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, adminAddr)),
		framework.WithPremine(adminAddr, targetAddr),
		framework.WithContractDeployerBlockListAdmin(adminAddr),
		framework.WithContractDeployerBlockListEnabled(otherAddr),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	// bytecode for an empty smart contract
	bytecode, err := hex.DecodeString("608060405234801561001057600080fd5b506103db806100206000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c80635e9f13f81461003b578063d78bca6914610059575b600080fd5b610043610089565b6040516100509190610217565b60405180910390f35b610073600480360381019061006e9190610263565b6100a1565b60405161008091906102a9565b60405180910390f35b73020000000000000000000000000000000000000081565b600080600073020000000000000000000000000000000000000073ffffffffffffffffffffffffffffffffffffffff16846040516024016100e29190610217565b6040516020818303038152906040527fd78bca69000000000000000000000000000000000000000000000000000000007bffffffffffffffffffffffffffffffffffffffffffffffffffffffff19166020820180517bffffffffffffffffffffffffffffffffffffffffffffffffffffffff838183161783525050505060405161016c9190610335565b6000604051808303816000865af19150503d80600081146101a9576040519150601f19603f3d011682016040523d82523d6000602084013e6101ae565b606091505b50915091506000818060200190518101906101c99190610378565b9050809350505050919050565b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b6000610201826101d6565b9050919050565b610211816101f6565b82525050565b600060208201905061022c6000830184610208565b92915050565b600080fd5b610240816101f6565b811461024b57600080fd5b50565b60008135905061025d81610237565b92915050565b60006020828403121561027957610278610232565b5b60006102878482850161024e565b91505092915050565b6000819050919050565b6102a381610290565b82525050565b60006020820190506102be600083018461029a565b92915050565b600081519050919050565b600081905092915050565b60005b838110156102f85780820151818401526020810190506102dd565b60008484015250505050565b600061030f826102c4565b61031981856102cf565b93506103298185602086016102da565b80840191505092915050565b60006103418284610304565b915081905092915050565b61035581610290565b811461036057600080fd5b50565b6000815190506103728161034c565b92915050565b60006020828403121561038e5761038d610232565b5b600061039c84828501610363565b9150509291505056fea264697066735822122035f391b5c3dbf9a5e31072167a4ada71e9ba762650849f6320bc6150fa45aa9564736f6c63430008110033")
	require.NoError(t, err)

	{
		// Step 0. Check the role of accounts
		expectRole(t, cluster, contracts.BlockListContractsAddr, adminAddr, addresslist.AdminRole)
		expectRole(t, cluster, contracts.BlockListContractsAddr, targetAddr, addresslist.NoRole)
		expectRole(t, cluster, contracts.BlockListContractsAddr, otherAddr, addresslist.EnabledRole)
	}

	{
		// Step 1. 'targetAddr' can send a normal transaction (non-contract creation).
		err := cluster.Transfer(t, target, types.ZeroAddress, big.NewInt(1)).Wait()
		require.NoError(t, err)
	}

	{
		// Step 2. 'targetAddr' **can** deploy a contract because it is not blacklisted.
		deployTxn := cluster.Deploy(t, target, bytecode)
		require.NoError(t, deployTxn.Wait())
		require.True(t, deployTxn.Succeed())
		require.True(t, cluster.ExistsCode(t, deployTxn.Receipt().ContractAddress))
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
		input, _ := addresslist.SetEnabledFunc.Encode([]interface{}{targetAddr})

		adminSetTxn := cluster.MethodTxn(t, admin, contracts.BlockListContractsAddr, input)
		require.NoError(t, adminSetTxn.Wait())
		expectRole(t, cluster, contracts.BlockListContractsAddr, targetAddr, addresslist.EnabledRole)
	}

	{
		// Step 5. 'targetAddr' **cannot** create contracts now as it's now blacklisted.
		deployTxn := cluster.Deploy(t, target, bytecode)
		require.NoError(t, deployTxn.Wait())
		require.True(t, deployTxn.Reverted())
		require.False(t, cluster.ExistsCode(t, deployTxn.Receipt().ContractAddress))
	}

	{
		// Step 6. 'targetAddr' cannot enable other accounts since it is not an admin
		// (The transaction fails)
		input, _ := addresslist.SetEnabledFunc.Encode([]interface{}{types.ZeroAddress})

		adminSetFailTxn := cluster.MethodTxn(t, target, contracts.BlockListContractsAddr, input)
		require.NoError(t, adminSetFailTxn.Wait())
		require.True(t, adminSetFailTxn.Failed())
		expectRole(t, cluster, contracts.BlockListContractsAddr, types.ZeroAddress, addresslist.NoRole)
	}
}

func TestE2E_AllowList_Transactions(t *testing.T) {
	// create two accounts, one for an admin sender and a second
	// one for a non-enabled account that will switch on-off between
	// both enabled and non-enabled roles.
	admin, _ := wallet.GenerateKey()
	target, _ := wallet.GenerateKey()
	other, _ := wallet.GenerateKey()

	adminAddr := types.Address(admin.Address())
	targetAddr := types.Address(target.Address())
	otherAddr := types.Address(other.Address())

	cluster := framework.NewTestCluster(t, 5,
		framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, adminAddr)),
		framework.WithPremine(adminAddr, targetAddr, otherAddr),
		framework.WithTransactionsAllowListAdmin(adminAddr),
		framework.WithTransactionsAllowListEnabled(otherAddr),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	// bytecode for an empty smart contract
	bytecode, _ := hex.DecodeString("608060405234801561001057600080fd5b506103db806100206000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c80635e9f13f81461003b578063d78bca6914610059575b600080fd5b610043610089565b6040516100509190610217565b60405180910390f35b610073600480360381019061006e9190610263565b6100a1565b60405161008091906102a9565b60405180910390f35b73020000000000000000000000000000000000000081565b600080600073020000000000000000000000000000000000000073ffffffffffffffffffffffffffffffffffffffff16846040516024016100e29190610217565b6040516020818303038152906040527fd78bca69000000000000000000000000000000000000000000000000000000007bffffffffffffffffffffffffffffffffffffffffffffffffffffffff19166020820180517bffffffffffffffffffffffffffffffffffffffffffffffffffffffff838183161783525050505060405161016c9190610335565b6000604051808303816000865af19150503d80600081146101a9576040519150601f19603f3d011682016040523d82523d6000602084013e6101ae565b606091505b50915091506000818060200190518101906101c99190610378565b9050809350505050919050565b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b6000610201826101d6565b9050919050565b610211816101f6565b82525050565b600060208201905061022c6000830184610208565b92915050565b600080fd5b610240816101f6565b811461024b57600080fd5b50565b60008135905061025d81610237565b92915050565b60006020828403121561027957610278610232565b5b60006102878482850161024e565b91505092915050565b6000819050919050565b6102a381610290565b82525050565b60006020820190506102be600083018461029a565b92915050565b600081519050919050565b600081905092915050565b60005b838110156102f85780820151818401526020810190506102dd565b60008484015250505050565b600061030f826102c4565b61031981856102cf565b93506103298185602086016102da565b80840191505092915050565b60006103418284610304565b915081905092915050565b61035581610290565b811461036057600080fd5b50565b6000815190506103728161034c565b92915050565b60006020828403121561038e5761038d610232565b5b600061039c84828501610363565b9150509291505056fea264697066735822122035f391b5c3dbf9a5e31072167a4ada71e9ba762650849f6320bc6150fa45aa9564736f6c63430008110033")

	{
		// Step 0. Check the role of both accounts
		expectRole(t, cluster, contracts.AllowListTransactionsAddr, adminAddr, addresslist.AdminRole)
		expectRole(t, cluster, contracts.AllowListTransactionsAddr, targetAddr, addresslist.NoRole)
		expectRole(t, cluster, contracts.AllowListTransactionsAddr, otherAddr, addresslist.EnabledRole)
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
		input, _ := addresslist.SetEnabledFunc.Encode([]interface{}{targetAddr})

		adminSetTxn := cluster.MethodTxn(t, admin, contracts.AllowListTransactionsAddr, input)
		require.NoError(t, adminSetTxn.Wait())
		expectRole(t, cluster, contracts.AllowListTransactionsAddr, targetAddr, addresslist.EnabledRole)
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
		input, _ := addresslist.SetEnabledFunc.Encode([]interface{}{types.ZeroAddress})

		adminSetFailTxn := cluster.MethodTxn(t, target, contracts.AllowListTransactionsAddr, input)
		require.NoError(t, adminSetFailTxn.Wait())
		require.True(t, adminSetFailTxn.Failed())
		expectRole(t, cluster, contracts.AllowListTransactionsAddr, types.ZeroAddress, addresslist.NoRole)
	}

	{
		// Step 6. 'adminAddr' sends a transaction to disable himself.
		input, _ := addresslist.SetNoneFunc.Encode([]interface{}{adminAddr})

		noneSetTxn := cluster.MethodTxn(t, admin, contracts.AllowListTransactionsAddr, input)
		require.NoError(t, noneSetTxn.Wait())
		require.True(t, noneSetTxn.Failed())
		expectRole(t, cluster, contracts.AllowListTransactionsAddr, adminAddr, addresslist.AdminRole)
	}
}

func TestE2E_BlockList_Transactions(t *testing.T) {
	// create two accounts, one for an admin sender and a second
	// one for a non-enabled account that will switch on-off between
	// both enabled and non-enabled roles.
	admin, _ := wallet.GenerateKey()
	target, _ := wallet.GenerateKey()
	other, _ := wallet.GenerateKey()

	adminAddr := types.Address(admin.Address())
	targetAddr := types.Address(target.Address())
	otherAddr := types.Address(other.Address())

	cluster := framework.NewTestCluster(t, 5,
		framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, adminAddr)),
		framework.WithPremine(adminAddr, targetAddr, otherAddr),
		framework.WithTransactionsBlockListAdmin(adminAddr),
		framework.WithTransactionsBlockListEnabled(otherAddr),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	{
		// Step 0. Check the role of both accounts
		expectRole(t, cluster, contracts.BlockListTransactionsAddr, adminAddr, addresslist.AdminRole)
		expectRole(t, cluster, contracts.BlockListTransactionsAddr, targetAddr, addresslist.NoRole)
		expectRole(t, cluster, contracts.BlockListTransactionsAddr, otherAddr, addresslist.EnabledRole)
	}

	{
		// Step 1. 'otherAddr' **cannot** send a normal transaction (non-contract creation) because it is blacklisted.
		otherTxn := cluster.Transfer(t, other, types.ZeroAddress, big.NewInt(1))
		require.NoError(t, otherTxn.Wait())
		require.True(t, otherTxn.Reverted())
	}

	{
		// Step 2. 'targetAddr' **can** send a normal transaction because it is not blacklisted.
		targetTxn := cluster.Transfer(t, target, types.ZeroAddress, big.NewInt(1))
		require.NoError(t, targetTxn.Wait())
		require.True(t, targetTxn.Succeed())
	}

	{
		// Step 3. 'targetAddr' cannot enable other accounts since it is not an admin
		// (The transaction fails)
		input, _ := addresslist.SetEnabledFunc.Encode([]interface{}{types.ZeroAddress})

		adminSetFailTxn := cluster.MethodTxn(t, target, contracts.BlockListTransactionsAddr, input)
		require.NoError(t, adminSetFailTxn.Wait())
		require.True(t, adminSetFailTxn.Failed())
		expectRole(t, cluster, contracts.BlockListTransactionsAddr, types.ZeroAddress, addresslist.NoRole)
	}

	{
		// Step 4. 'adminAddr' sends a transaction to enable 'targetAddr'.
		input, _ := addresslist.SetEnabledFunc.Encode([]interface{}{targetAddr})

		adminSetTxn := cluster.MethodTxn(t, admin, contracts.BlockListTransactionsAddr, input)
		require.NoError(t, adminSetTxn.Wait())
		expectRole(t, cluster, contracts.BlockListTransactionsAddr, targetAddr, addresslist.EnabledRole)
	}

	{
		// Step 5. 'targetAddr' **cannot** send a normal transaction because it is blacklisted.
		targetTxn := cluster.Transfer(t, target, types.ZeroAddress, big.NewInt(1))
		require.NoError(t, targetTxn.Wait())
		require.True(t, targetTxn.Reverted())
	}
}

func TestE2E_AddressLists_Bridge(t *testing.T) {
	// create two accounts, one for an admin sender and a second
	// one for a non-enabled account that will switch on-off between
	// both enabled and non-enabled roles.
	admin, _ := wallet.GenerateKey()
	target, _ := wallet.GenerateKey()
	other, _ := wallet.GenerateKey()

	adminAddr := types.Address(admin.Address())
	targetAddr := types.Address(target.Address())
	otherAddr := types.Address(other.Address())

	cluster := framework.NewTestCluster(t, 5,
		framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, adminAddr)),
		framework.WithPremine(adminAddr, targetAddr, otherAddr),
		framework.WithBridgeAllowListAdmin(adminAddr),
		framework.WithBridgeAllowListEnabled(otherAddr),
		framework.WithBridgeBlockListAdmin(adminAddr),
		framework.WithBridgeBlockListEnabled(otherAddr),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	{
		// Step 0. Check the role of both accounts
		expectRole(t, cluster, contracts.AllowListBridgeAddr, adminAddr, addresslist.AdminRole)
		expectRole(t, cluster, contracts.AllowListBridgeAddr, targetAddr, addresslist.NoRole)
		expectRole(t, cluster, contracts.AllowListBridgeAddr, otherAddr, addresslist.EnabledRole)
		expectRole(t, cluster, contracts.BlockListBridgeAddr, adminAddr, addresslist.AdminRole)
		expectRole(t, cluster, contracts.BlockListBridgeAddr, targetAddr, addresslist.NoRole)
		expectRole(t, cluster, contracts.BlockListBridgeAddr, otherAddr, addresslist.EnabledRole)
	}
}
