The `IChildERC1155Predicate` interface is designed to work with the ERC1155-compliant token on the L2 side of the childchain. It handles the minting, burning, and state management for child tokens. This user guide will explain how to interact with the functions provided by the `IChildERC1155Predicate` interface.

## Functions

### initialize()

This function initializes the predicate with the required parameters.

#### Parameters

- `newL2StateSender` (address): The L2 state sender address.
- `newStateReceiver` (address): The state receiver address.
- `newRootERC721Predicate` (address): The root ERC721 predicate address.
- `newChildTokenTemplate` (address): The child token template address.

#### Usage

To initialize the `IChildERC1155Predicate` instance, call the `initialize()` function with the required parameters:

```solidity
IChildERC1155Predicate.instance.initialize(newL2StateSender, newStateReceiver, newRootERC721Predicate, newChildTokenTemplate);
```

### onStateReceive()

This function receives the state data from the main chain.

#### Parameters

- `id` (uint256): An identifier for the state data.
- `sender` (address): The sender address.
- `data` (bytes): The state data.

#### Usage

To handle state data received from the main chain, the predicate should call the `onStateReceive()` function with the required parameters:

```solidity
IChildERC1155Predicate.instance.onStateReceive(id, sender, data);
```

### withdraw()

This function allows the user to withdraw their child tokens to the main chain.

#### Parameters

- `childToken` (IChildERC1155): The child token contract.
- `tokenId` (uint256): The token identifier.
- `amount` (uint256): The amount of tokens to withdraw.

#### Usage

To withdraw child tokens to the main chain, call the `withdraw()` function with the required parameters:

```solidity
IChildERC1155Predicate.instance.withdraw(childToken, tokenId, amount);
```

### withdrawTo()

This function allows the user to withdraw their child tokens to a specific address on the main chain.

#### Parameters

- `childToken` (IChildERC1155): The child token contract.
- `receiver` (address): The address to receive the tokens on the main chain.
- `tokenId` (uint256): The token identifier.
- `amount` (uint256): The amount of tokens to withdraw.

#### Usage

To withdraw child tokens to a specific address on the main chain, call the `withdrawTo()` function with the required parameters:

```solidity
IChildERC1155Predicate.instance.withdrawTo(childToken, receiver, tokenId, amount);
```
