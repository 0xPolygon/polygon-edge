package e2e

import (
	"fmt"
	"testing"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state/runtime/allowlist"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/compiler"
	"github.com/umbracle/ethgo/wallet"
)

const (
	statusFailed  = uint64(0)
	statusSucceed = uint64(1)
)

var allowListE2EContractCode = `// SPDX-License-Identifier: MIT
pragma solidity ^0.5.5;

interface AllowListInterface {
    function readAllowList() external view;
}

contract AllowListProxy {
	AllowListInterface allowlist = AllowListInterface(0x0200000000000000000000000000000000000000);

	function readAllowList(address addr) public view returns (uint256) {
		allowlist.readAllowList();
		return 1;
	}
}
`

var allowListE2EContract *compiler.Artifact

func init() {
	output, err := compiler.NewSolidityCompiler("solc").CompileCode(allowListE2EContractCode)
	if err != nil {
		panic(fmt.Sprintf("BUG: failed to compile sm: %v", err))
	}

	allowListE2EContract = output.Contracts["<stdin>:AllowListProxy"]
}

func TestAllowList_ContractDeployment(t *testing.T) {
	// create two accounts, one for an admin sender and a second
	// one for a non-enabled account that will switch on-off between
	// both enabled and non-enabled roles.

	admin, _ := wallet.GenerateKey()
	target, _ := wallet.GenerateKey()

	adminAddr := types.Address(admin.Address())
	targetAddr := types.Address(target.Address())

	cluster := framework.NewTestCluster(t, 3,
		framework.WithPremine(adminAddr, targetAddr),
		framework.WithAllowList(adminAddr),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	bytecode, _ := hex.DecodeString(allowListE2EContract.Bin)

	/*
		expectRole := func(addr types.Address, role allowlist.Role) {
			out := cluster.Call(t, allowlist.AllowListContractsAddr, allowlist.ReadAllowListFunc, addr)
			require.Equal(t, role.Uint64(), out["0"].(*big.Int).Uint64())
		}
	*/

	/*
		{
			// Step 0. Check the role of both accounts
			expectRole(adminAddr, allowlist.AdminRole)
			expectRole(targetAddr, allowlist.NoRole)
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
			require.Equal(t, statusFailed, deployTxn.Receipt().Status)
			require.Equal(t, deployTxn.Txn().Gas, deployTxn.Receipt().GasUsed)
			require.False(t, cluster.ExistsCode(t, deployTxn.Receipt().ContractAddress))
		}

	*/

	var deployedContractAddr types.Address

	{
		// Step 3. 'adminAddr' can create contracts
		deployTxn := cluster.Deploy(t, admin, bytecode)
		require.NoError(t, deployTxn.Wait())
		require.Equal(t, statusSucceed, deployTxn.Receipt().Status)
		require.True(t, cluster.ExistsCode(t, deployTxn.Receipt().ContractAddress))

		deployedContractAddr = types.Address(deployTxn.Receipt().ContractAddress)
	}

	/*
		{
			// Step 4. 'adminAddr' sends a transaction to enable 'targetAddr'.
			input, _ := allowlist.SetEnabledSignatureFunc.Encode([]interface{}{targetAddr})

			adminSetTxn := cluster.MethodTxn(t, admin, allowlist.AllowListContractsAddr, input)
			require.NoError(t, adminSetTxn.Wait())
			expectRole(targetAddr, allowlist.EnabledRole)
		}

		{
			// Step 5. 'targetAddr' can create contracts now.
			deployTxn := cluster.Deploy(t, target, bytecode)
			require.NoError(t, deployTxn.Wait())
			require.Equal(t, statusSucceed, deployTxn.Receipt().Status)
			require.True(t, cluster.ExistsCode(t, deployTxn.Receipt().ContractAddress))
		}

		{
			// Step 6. 'targetAddr' cannot enable other accounts since it is not an admin
			// (The transaction reverts and fails)
			input, _ := allowlist.SetEnabledSignatureFunc.Encode([]interface{}{types.ZeroAddress})

			adminSetFailTxn := cluster.MethodTxn(t, target, allowlist.AllowListContractsAddr, input)
			require.NoError(t, adminSetFailTxn.Wait())
			require.Equal(t, statusFailed, adminSetFailTxn.Receipt().Status)
			expectRole(types.ZeroAddress, allowlist.NoRole)
		}
	*/

	fmt.Println("_ CONTRACT _", deployedContractAddr)

	{
		// Step 7. The allowlist contract is accessible from another smart contract
		out := cluster.Call(t, deployedContractAddr, allowlist.ReadAllowListFunc, adminAddr)
		fmt.Println(out["0"])
	}
}
