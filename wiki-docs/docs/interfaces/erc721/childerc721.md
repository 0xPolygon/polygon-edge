The `IChildERC721` interface is designed to work with ERC721-compliant NFT tokens on a childchain. It provides functionality for minting, burning, and managing NFTs. This user guide will explain how to interact with the functions provided by the `IChildERC721` interface.

## Functions

### initialize()

This function initializes the child token contract with the root token address, name, and symbol.

#### Parameters

- `rootToken_` (address): The root token address.
- `name_` (string): The name of the token.
- `symbol_` (string): The symbol of the token.

#### Usage

To initialize the IChildERC721 instance, call the `initialize()` function with the required parameters:

```solidity
IChildERC721.instance.initialize(rootToken_, name_, symbol_);
```

### predicate()

This function returns the predicate address controlling the child token.

#### Usage

To get the predicate address, call the predicate() function:

```solidity
address predicateAddress = IChildERC721.instance.predicate();
```

### rootToken()

This function returns the address of the token on the rootchain.

#### Usage

To get the root token address, call the `rootToken()` function:

```solidity
address rootTokenAddress = IChildERC721.instance.rootToken();
```

### mint()

This function mints an NFT token to a specific address.

#### Parameters

- `account` (address): The recipient address.
- `tokenId` (uint256): The token identifier.

#### Usage

To mint an NFT token to a specific address, call the `mint()` function with the required parameters:

```solidity
bool success = IChildERC721.instance.mint(account, tokenId);
```

### mintBatch()

This function mints multiple NFT tokens in one transaction.

#### Parameters

- `accounts` (address[]): An array of recipient addresses.
- `tokenIds` (uint256[]): An array of token identifiers.

#### Usage

To mint multiple NFT tokens in one transaction, call the `mintBatch()` function with the required parameters:

```solidity
bool success = IChildERC721.instance.mintBatch(accounts, tokenIds);
```

### burn()

This function burns an NFT token from a specific address.

#### Parameters

- `account` (address): The address to burn the NFT from.
- `tokenId` (uint256): The token identifier.

#### Usage

To burn an NFT token from a specific address, call the `burn()` function with the required parameters:

```solidity
bool success = IChildERC721.instance.burn(account, tokenId);
```

### burnBatch()

This function burns multiple NFT tokens from a specific address in one transaction.

#### Parameters

- `account` (address): The address to burn the NFTs from.
- `tokenIds` (uint256[]): An array of token identifiers.

#### Usage

To burn multiple NFT tokens from a specific address in one transaction, call the `burnBatch()` function with the required parameters:

```solidity
bool success = IChildERC721.instance.burnBatch(account, tokenIds);
```
