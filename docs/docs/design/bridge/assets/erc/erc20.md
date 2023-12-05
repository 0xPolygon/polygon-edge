Edge provides developers with two standards for creating ERC-20 fungible tokens on their networks: `NativeERC20` and `ChildERC20`. These standards are based on the widely-used ERC-20 standard and offer similar functionality, including the ability to transfer tokens between addresses, approve others to spend tokens on your behalf, and check your balance. However, there are significant differences in their deployment and management.

## ChildERC20

The [`ChildERC20`](../../../../interfaces/erc20/childerc20.md) token standard is used for tokens mapping and enables developers to create tokens on both the Edge-powered chain and the associated rootchain. To create a new `ChildERC20` token, developers can use the `ChildERC20` contract as a template and deploy it on their Edge-powered chain. The contract requires a name, symbol, and number of decimals to determine the minimum unit of the token.

`ChildERC20` tokens are minted and burned on the Edge-powered chain through the corresponding [`ERC20Predicate`](../../../../interfaces/erc20/childerc20-predicate.md) contract. This ensures that the supply of tokens on the rootchain and the Edge-powered chain remains in sync. The `ERC20Predicate` contract also allows for the transfer of tokens between the two networks using the native bridge.

## NativeERC20

The [`NativeERC20`](../../../../interfaces/erc20/native-erc20.md) token standard represents native tokens on Edge and offers fast and inexpensive transactions. It is deployed only on the Edge-powered chain and relies on the native transfer precompile to make transfers. The `NativeERC20` tokens can be minted and burned by the associated predicate contract.

In addition, developers can use the `NativeERC20Mintable` contract to create and manage `NativeMintable` tokens, which are fungible tokens that represent assets on the Edge-powered chain. These tokens can be managed through the native bridge contract and transferred between an Edge-powered chain and rootchain networks.

## Deposits and Withdrawals

The deposit and withdrawal functionality plays a critical role in bridging ERC-20 tokens between a rootchain and an Edge-powered chain. 

When a user wants to deposit ERC-20 tokens into an Edge-powered chain, they call the `deposit` function. This function maps the root token to a child token and then mints the equivalent amount of child tokens to the user's address on the Edge-powered chain. By doing this, the ERC-20 tokens are effectively transferred from the rootchain to the Edge-powered chain. The user can then use these tokens on the Edge-powered chain or transfer them to other addresses on the network.

On the other hand, when a user wants to withdraw ERC-20 tokens from an Edge-powered chain to the rootchain, they call the `withdraw` function. This function burns the equivalent amount of child tokens on the Edge-powered chain and then triggers a function on the rootchain that transfers the equivalent amount of root tokens to the user's address on the rootchain. By doing this, the ERC-20 tokens are effectively transferred from the Edge-powered chain back to the rootchain.

Both the `deposit` and `withdraw` functions emit events that log the amount and token involved in the transaction. This provides transparency and allows users to track the movement of their tokens between the two networks.

### EIP1559Burn

The dynamic gas fee mechanism implements a base fee per gas to improve transaction fee predictability and efficiency. The base fee adjusts dynamically based on gas target deviations and is burned within the protocol.

- **Burn Contract**: The burn contract parameter specifies the contract address where fees will be sent and utilized. To enable the London hard fork and set the burn contract address, use the `--burn-contract` flag in the genesis command with the following format: `<block>:<address>`.

- **Genesis Base Fee**: The genesis base fee parameter sets the initial base fee (in GWEI) for the genesis block. Include this value in the genesis.json file during the genesis command. By default, the genesis base fee is set to 1 GWEI.

- **Genesis Base FeeEM**: The genesis base fee elasticity multiplier determines the base fee for blocks following the genesis block. It is initially set to 2. If the London hard fork is disabled (i.e., when the burn contract is not set), the base fee remains the same as the genesis base fee.

- **Burn Contract Parameter Revision**: The burn contract address can be adjusted during a node reboot, which is achieved by modifying the `burnContract` parameter in the genesis.json file. It's crucial to comprehend, however, that merely identifying the burn contract address does not implicitly activate the EIP-1559 feature. If the EIP-1559 was not initialized during genesis, a series of procedures are required to activate it subsequently:

- The burn contract should be properly deployed onto the Edge-powered chain.
- The genesis file must detail the burn contracts map, incorporating both the block number and the burn contract address.
- The base fee and base fee elasticity multiplier are required to be manually defined in the genesis file.
- Finally, the EIP1559 fork must be activated.
