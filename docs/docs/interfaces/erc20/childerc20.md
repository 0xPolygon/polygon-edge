The `IChildERC20` interface extends the `IERC20MetadataUpgradeable` interface to provide additional token functionality on a childchain or layer 2 solution. This guide explains how to interact with the interface and its methods.

## Overview

The `IChildERC20` interface includes methods for:

- Initializing the token
- Retrieving the predicate and root token addresses
- Minting and burning tokens

## Initializing the Token

To initialize a child token, call the initialize function with the following parameters:

- `rootToken_`: The address of the root token on the main chain.
- `name_`: The name of the token.
- `symbol_`: The symbol of the token.
- `decimals_`: The number of decimals used to define the smallest unit of the token.

Example

```solidity
childToken.initialize(rootTokenAddress, "My Token", "MTK", 18);
```

## Retrieving the Predicate and Root Token Addresses

To get the predicate and root token addresses, call the `predicate` and `rootToken` functions respectively.

Example:

```solidity
address predicateAddress = childToken.predicate();
address rootTokenAddress = childToken.rootToken();
```

## Minting and Burning Tokens

The `mint` and `burn` functions can only be called by the predicate address, which typically represents a bridge contract responsible for managing the token's supply between the main chain and the childchain.

### Minting Tokens

To mint tokens, call the mint function with the following parameters:

- `account`: The user's address to mint the tokens.
- `amount`: The amount of tokens to mint to the account.

```solidity
bool success = childToken.mint(userAddress, 100 * 10**18);
```

## Burning Tokens

To burn tokens, call the burn function with the following parameters:

- `account`: The user's address to burn the tokens.
- `amount`: The amount of tokens to burn from the account.

Example:

```solidity
bool success = childToken.burn(userAddress, 50 * 10**18);
```

> Note: The predicate address should only call the mint and burn functions. If you're a developer integrating with a child token, you typically won't need to interact with these functions directly. Instead, you'll interact with the bridge or the main chain's root token contract to move tokens between chains.
