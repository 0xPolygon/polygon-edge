The `INativeERC20` interface represents a native ERC20 token on the L2 side of the childchain. It allows the minting and burning of tokens under the control of a predicate address. This user guide will explain how to interact with the functions provided by the `INativeERC20` interface.

## Functions

### initialize()

This function sets the values for the predicate, rootToken, name, symbol, and decimals.

#### Parameters

- `predicate_` (address): The address of the predicate controlling the child token.
- `rootToken_` (address): The address of the root token on the mainchain.
- `name_` (string): The token's name.
- `symbol_` (string): The token's symbol.
- `decimals_` (uint8): The number of decimals for the token.
- `tokenSupply_` (uint256): Initial total supply of token.

#### Usage

To initialize the INativeERC20 instance, call the `initialize()` function with the required parameters:

```solidity
INativeERC20.instance.initialize(predicate, rootToken, name, symbol, decimals);
```

### predicate()

This function returns the predicate address controlling the child token.

#### Usage

To get the predicate address, call the `predicate()` function:

```solidity
const predicateAddress = INativeERC20.instance.predicate();
```

### rootToken()

This function returns the root token address.

#### Usage

To get the root token address, call the `rootToken()` function:

```solidity
const rootTokenAddress = INativeERC20.instance.rootToken();
```

### mint()

This function mints an amount of tokens to a particular address. The predicate address can only call it.

#### Parameters

- `account` (address): The user's account to mint the tokens.
- `amount` (uint256): The token amount to mint to the account.

#### Usage

To mint tokens to an address, the predicate should call the `mint()` function with the required parameters:

```solidity
INativeERC20.instance.mint(account, amount);
```

### burn()

This function burns an amount of tokens from a particular address. The predicate address can only call it.

#### Parameters

- `account` (address): The user's account to burn the tokens from.
- `amount` (uint256): The amount of tokens to burn from the account.

#### Usage

To burn tokens from an address, the predicate should call the `burn()` function with the required parameters:

```solidity
INativeERC20.instance.burn(account, amount);
```

These functions enable interaction with native ERC20 tokens on the L2 side of the childchain. Developers can use these functions to mint, burn, and retrieve information about native ERC20 tokens.
