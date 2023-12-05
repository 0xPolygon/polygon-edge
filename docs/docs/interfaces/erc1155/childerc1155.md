The `IChildERC1155` interface represents an ERC1155-compliant token on the L2 side of the childchain. It allows the minting and burning of tokens under the control of a predicate address. This user guide will explain how to interact with the functions provided by the `IChildERC1155` interface.

## Functions

### initialize()

This function sets the values for the rootToken, name, and uri. The value for rootToken is immutable and can only be set once during initialization.

#### Parameters

- `rootToken_` (address): The address of the token on the rootchain.
- `name_` (string): The token's name.
- `uri_` (string): The token's metadata URI.

#### Usage

To initialize the IChildERC1155 instance, call the initialize() function with the required parameters:

```solidity
IChildERC1155.instance.initialize(rootToken, name, uri);
```

### predicate()

This function returns the predicate address controlling the child token.

#### Usage

To get the predicate address, call the predicate() function:

```solidity
const predicateAddress = IChildERC1155.instance.predicate();
```

### rootToken()

This function returns the address of the token on the rootchain.

#### Usage

To get the address of the token on the rootchain, call the rootToken() function:

```solidity
const rootTokenAddress = IChildERC1155.instance.rootToken();
```

### mint()

This function mints an NFT token to a particular address. The predicate address can only call it.

#### Parameters

- `account` (address): The user's account to mint the tokens.
- `id` (uint256): The index of NFT to mint to the account.
- `amount` (uint256): The amount of NFT to mint.

#### Usage

To mint an NFT token to an address, the predicate should call the mint() function with the required parameters:

```solidity
bool success = IChildERC1155.instance.mint(account, id, amount);
```

### mintBatch()

This function mints multiple NFTs to one address.

#### Parameters

- `accounts` (address[]): An array of addresses to mint each NFT to.
- `tokenIds` (uint256[]): An array of indexes of the NFTs to be minted.
- `amounts` (uint256[]): An array of the amount of each NFT to be minted.

#### Usage

To mint multiple NFTs to one address, call the mintBatch() function with the required parameters:

```solidity
bool success = IChildERC1155.instance.mintBatch(accounts, tokenIds, amounts);
```

### burn()

This function burns an NFT token from a particular address. The predicate address can only call it.

#### Parameters

- `from` (address): The user's account to burn the tokens from.
- `id` (uint256): The index of NFT to burn from the account.
- `amount` (uint256): The amount of NFT to burn.

#### Usage

To burn an NFT token from an address, the predicate should call the burn() function with the required parameters:

```solidity
bool success = IChildERC1155.instance.burn(from, id, amount);
```

### burnBatch()

This function burns multiple NFTs from one address.

#### Parameters

- `from` (address): The address to burn NFTs from.
- `tokenIds` (uint256[]): An array of indexes of the NFTs to be burned.
- `amounts` (uint256[]): An array of the amount of each N

#### Usage

To burn multiple NFTs from one address, call the burnBatch() function with the required parameters:

```solidity
bool success = IChildERC1155.instance.burnBatch(from, tokenIds, amounts);
```
