The `IRootERC20Predicate` interface is designed to work with ERC20 tokens on a rootchain. It provides functionality for depositing and mapping ERC20 tokens between root and childchains. This user guide will explain how to interact with the functions provided by the `IRootERC20Predicate` interface.

## Functions

### deposit()

This function deposits tokens from the depositor to themselves on the childchain.

#### Parameters

- `rootToken` (IERC20Metadata): The root token being deposited.
- `amount` (uint256): The amount to deposit.

#### Usage

To deposit tokens from the depositor to themselves on the childchain, call the `deposit()` function with the required parameters:

```solidity
IRootERC20Predicate.instance.deposit(rootToken, amount);
```

### depositTo()

This function deposits tokens from the depositor to another address on the childchain.

#### Parameters

- `rootToken` (IERC20Metadata): The root token being deposited.
- `receiver` (address): The address of the receiver on the childchain.
- `amount` (uint256): The amount to deposit.

#### Usage

To deposit tokens from the depositor to another address on the childchain, call the `depositTo()` function with the required parameters:

```solidity
IRootERC20Predicate.instance.depositTo(rootToken, receiver, amount);
```

### mapToken()

This function is used for token mapping.

#### Parameters

rootToken (IERC20Metadata): The address of the root token to map.

#### Usage

The mapToken() function is called internally on deposit if the token is not mapped already.
