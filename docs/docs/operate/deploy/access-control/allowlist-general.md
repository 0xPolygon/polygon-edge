
## Understanding ACL and Address Roles

ACLs or Access Control Lists are a way of managing permissions in your Edge-powered chain. In this context, an ACL is essentially a list of addresses and their corresponding roles.

:::info Keep in mind

- **Enabling Lists**: Allowlists and blocklists are enabled or disabled exclusively during network initialization through the genesis command. Changes to the configuration cannot be made dynamically.
- **Admin Role**: To enable a list, an admin role must be set in the genesis command. The admin manages the list and can only be specified during the network's initial setup.
- **Exclusive Enablement**: It is not valid to enable both allowlists and blocklists for a given list type. If both lists are set, the allowlist takes precedence, and the blocklist is ignored.
- **System Transaction Address**: The system transaction address (0xffffFFFfFFffffffffffffffFfFFFfffFFFfFFfE) is excluded from allowlist and blocklist validation. It is always allowed to perform actions and is not subject to list checks.
- **Impact on Validators and System Transactions**: The impact of allowlists and blocklists on validators and system transactions can vary depending on network implementation.

:::

## ABI Specification

### Overview

Below is an overview of the ABI for the ACL in Edge:

```shell
function setAdmin(address)
function setEnabled(address)
function setNone(address)
function readAddressList(address) returns (uint256)
```

### Functions

**Roles can be one of three types:**

- `NoRole`: The address has no permissions.
- `EnabledRole`: The address has some permissions.
- `AdminRole`: The address has all permissions and can change the roles of other addresses.

**For more information about how ACLs work in Edge, check out the overview guide [<ins>here</ins>](../../../design/runtime/allowlist.md).**

```go
NoRole      Role = Role(types.StringToHash("..."))
EnabledRole Role = Role(types.StringToHash("..."))
AdminRole   Role = Role(types.StringToHash("..."))
```

- **Function Signatures**: The module provides functions to set or query roles:
  - Set an address as an admin.
  - Enable an address.
  - Remove any role from an address.
  - Read the role of an address.

```go
SetAdminFunc        = abi.MustNewMethod("function setAdmin(address)")
SetEnabledFunc      = abi.MustNewMethod("function setEnabled(address)")
SetNoneFunc         = abi.MustNewMethod("function setNone(address)")
ReadAddressListFunc = abi.MustNewMethod("function readAddressList(address) returns (uint256)")
```

### Key Functionalities

- **Creating an AddressList**: An instance of the AddressList is created with a reference to the state and the contract's address.

```go
func NewAddressList(state stateRef, addr types.Address) *AddressList {...}
```

- **Running the Contract**: The `Run` function decodes the input to determine the function being called and executes it.

```go
func (a *AddressList) Run(c *runtime.Contract, host runtime.Host, _ *chain.ForksInTime) *runtime.ExecutionResult {...}
```

- **Setting and Getting Roles**: The module provides functions to assign a role to an address and to fetch the role of a given address.

```go
func (a *AddressList) SetRole(addr types.Address, role Role) {...}
func (a *AddressList) GetRole(addr types.Address) Role {...}
```

### How It Works

1. A call to the `AddressList` triggers the `Run` function.
2. The function decodes the input to identify the function being called.
3. If querying a role, the role of the provided address is returned.
4. For modifying roles, checks are in place to ensure:
   - The caller has enough gas.
   - The call isn't static (read-only).
   - Only admins can modify roles.
   - Admins can't remove their own admin role.

## Interacting with an ACL

Edge ACLs are implemented as precompiles, which are built-in contracts within the Edge client. You can interact with these ACL precompiles using libraries like ether.js and web3.js.

To interact with the ACL precompiles using an external library or client, follow these steps:

1. Obtain the ABI (Application Binary Interface) for the specific ACL precompile you want to interact with. The ABI contains the definitions of the precompile's functions, including their parameters and return types.

2. Create a contract instance for the ACL precompile using the ABI and the provider connected to the Edge-powered chain.

3. Use the contract instance to call the functions defined in the precompile's ABI. These functions allow you to manage the ACL by adding or removing accounts, modifying roles, or performing other ACL-related operations.

## Example Interaction with ethers.js

### Prerequisites

- Install ethers.js in your project using npm or yarn: `npm i --save ethers`.
- Ensure you have a wallet and some gas for transaction fees.
- Obtain the address and ABI of the ACL smart contract you want to interact with.

:::note

- These operations should be performed by an account with the `AdminRole`.
- Make sure to replace the placeholders with your actual values.
- The await keyword must be used in an async function.
- Be mindful of transaction fees (gas costs) when interacting with the network.

:::

### Sample Workflow

Create a contract instance for the ACL precompile using the ABI and the provider connected to the network.
Please note that these examples are provided as guidance and assume the usage of ether.js, with Alice and Bob used as placeholders for specific agents.

```javascript
const ethers = require('ethers');

// Connect to the provider (e.g., local node, Infura, Alchemy, etc.)
const provider = new ethers.providers.JsonRpcProvider('https://rpc-endpoint.io');

const ACLInterface = [{...}]; // Replace with the ABI of the Access List precompile

// Contract Deployer Allow List
const contractDeployerAlowListAddress = '0xContractDeployerAllowListAddress';
const contractDeployerAllowList = new ethers.Contract(contractDeployerAddress, ACLInterface, provider);

// Transactions Allow List
const transactionsAllowListAddress = '0xTransactionsAllowListAddress';
const transactionsAllowList = new ethers.Contract(transactionsAllowListAddress, ACLInterface, provider);

// Transactions Block List
const transactionsBlockListAddress = '0xTransactionsBlockListAddress';
const transactionsBlockList = new ethers.Contract(transactionsBlockListAddress, ACLInterface, provider);

// Bridge Allow List
const bridgeAllowListAddress = '0xBridgeAllowListAddress';
const bridgeAllowList = new ethers.Contract(bridgeAllowListAddress, ACLInterface, provider);

// Bridge Block List
const bridgeBlockListAddress = '0xBridgeBlockListAddress';
const bridgeBlockList = new ethers.Contract(bridgeBlockListAddress, ACLInterface, provider);
```

#### Interact with the ContractDeployer ACL

```javascript
// Example interaction with the Contract Deployer ACL

const addrAlice = '0xAliceAddress'; // Replace with Alice's address
const addrBob = '0xBobAddress'; // Replace with Bob's address

// Add Alice's address to the contract deployer allowlist
const allowlistContractDeployerTx1 = contractDeployerAllowList.setEnabled(addrAlice);
allowlistContractDeployerTx1.wait();

// Remove Alice's address from the contract deployer allowlist
const allowlistContractDeployerTx2 = contractDeployerAllowList.setNone(addrAlice);
allowlistContractDeployerTx2.wait();

// Add Bob's address to the contract deployer allowlist
const allowlistContractDeployerTx3 = contractDeployerAllowList.setEnabled(addrBob);
allowlistContractDeployerTx3.wait();

// Remove Bob's address from the contract deployer allowlist
const allowlistContractDeployerTx4 = contractDeployerAllowList.setNone(addrBob);
allowlistContractDeployerTx4.wait();
```

#### Interact with Transactions ACL

```javascript
// Add Bob's address to the transaction allowlist
const allowlistTxTx1 = transactionsAllowList.setEnabled(addrBob);
allowlistTxTx1.wait();

// Remove Bob's address from the transaction allowlist
const allowlistTxTx2 = transactionsAllowList.setNone(addrBob);
allowlistTxTx2.wait();

// Add Alice's address to the transaction blocklist
const blocklistTxTx1 = transactionsBlockList.setEnabled(addrAlice);
blocklistTxTx1.wait();

// Remove Alice's address from the transaction blocklist
const blocklistTxTx2 = transactionsBlockList.setNone(addrAlice);
blocklistTxTx2.wait();
```

#### Interact with the Bridge ACL

```javascript
// Add Alice's address to the bridge allowlist
const allowlistBridgeTx1 = bridgeAllowList.setEnabled(addrAlice);
allowlistBridgeTx1.wait();

// Remove Alice's address from the bridge allowlist
const allowlistBridgeTx2 = bridgeAllowList.setNone(addrAlice);
allowlistBridgeTx2.wait();

// Add Alice's address to the bridge blocklist
const blocklistBridgeTx1 = bridgeBlockList.setEnabled(addrAlice);
blocklistBridgeTx1.wait();

// Remove Alice's address from the bridge blocklist
const blocklistBridgeTx2 = bridgeBlockList.setNone(addrAlice);
blocklistBridgeTx2.wait();
```
