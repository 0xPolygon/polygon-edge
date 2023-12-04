The `IChildERC721Predicate` interface is designed to work in conjunction with the `IChildERC721` interface to enable the withdrawal of ERC721-compliant NFT tokens from a childchain back to the rootchain. This user guide will explain how to interact with the functions provided by the `IChildERC721Predicate` interface.

## Functions

### initialize()

This function initializes the predicate contract with required parameters.

#### Parameters

- `newL2StateSender` (address): The address of the L2 state sender.
- `newStateReceiver` (address): The address of the state receiver.
- `newRootERC721Predicate` (address): The address of the root ERC721 predicate.
- `newChildTokenTemplate` (address): The address of the child token template.

#### Usage

To initialize the `IChildERC721Predicate` instance, call the `initialize()` function with the required parameters:

```solidity
IChildERC721Predicate.instance.initialize(newL2StateSender, newStateReceiver, newRootERC721Predicate, newChildTokenTemplate);
```

### onStateReceive()

This function is called when the predicate receives a state update from the L2 state sender.

#### Parameters

- `id` (uint256): An identifier, not used in this implementation.
- `sender` (address): The address of the sender.
- `data` (bytes): The calldata to be processed.

#### Usage

The `onStateReceive()` function is not intended to be called directly by developers. It is automatically called when the predicate receives a state update from the L2 state sender.

### withdraw()

This function withdraws an NFT token to the original owner.

#### Parameters

- `childToken` (IChildERC721): The child token contract instance.
- `tokenId` (uint256): The token identifier.

#### Usage

To withdraw an NFT token to the original owner, call the `withdraw()` function with the required parameters:

```solidity
IChildERC721Predicate.instance.withdraw(childToken, tokenId);
```

### withdrawTo()

This function withdraws an NFT token to a specified address.

#### Parameters

- `childToken` (IChildERC721): The child token contract instance.
- `receiver` (address): The address of the recipient.
- `tokenId` (uint256): The token identifier.

#### Usage

To withdraw an NFT token to a specified address, call the `withdrawTo()` function with the required parameters:

```solidity
IChildERC721Predicate.instance.withdrawTo(childToken, receiver, tokenId);
```

### withdrawBatch()

This function withdraws multiple NFT tokens to specified addresses in one transaction.

#### Parameters

- `childToken` (IChildERC721): The child token contract instance.
- `receivers` (address[]): An array of recipient addresses.
- `tokenIds` (uint256[]): An array of token identifiers.

#### Usage

To withdraw multiple NFT tokens to specified addresses in one transaction, call the `withdrawBatch()` function with the required parameters:

```solidity
IChildERC721Predicate.instance.withdrawBatch(childToken, receivers, tokenIds);
```
