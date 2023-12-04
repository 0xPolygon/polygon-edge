The `IRootERC721Predicate` interface is designed to work with ERC721-compliant NFT tokens on a rootchain. It provides functionality for depositing and mapping ERC721 tokens between root and childchains. This user guide will explain how to interact with the functions provided by the `IRootERC721Predicate` interface.

## Functions

### deposit()

This function deposits tokens from the depositor to themselves on the childchain.

#### Parameters

- `rootToken` (IERC721Metadata): The root token being deposited.
- `tokenId` (uint256): The index of the NFT to deposit.

#### Usage

To deposit tokens from the depositor to themselves on the childchain, call the `deposit()` function with the required parameters:

```solidity
IRootERC721Predicate.instance.deposit(rootToken, tokenId);
```

### depositTo()

This function deposits tokens from the depositor to another address on the childchain.

#### Parameters

- `rootToken` (IERC721Metadata): The root token being deposited.
- `receiver` (address): The address of the receiver on the childchain.
- `tokenId` (uint256): The index of the NFT to deposit.

#### Usage

To deposit tokens from the depositor to another address on the childchain, call the depositTo() function with the required parameters:

```solidity
IRootERC721Predicate.instance.depositTo(rootToken, receiver, tokenId);
```

### depositBatch()

This function deposits tokens from the depositor to other addresses on the childchain.

#### Parameters

- `rootToken` (IERC721Metadata): The root token being deposited.
- `receivers` (address[]): An array of addresses of the receivers on the childchain.
- `tokenIds` (uint256[]): An array of indices of the NFTs to deposit.

#### Usage

To deposit tokens from the depositor to other addresses on the childchain, call the `depositBatch()` function with the required parameters:

```solidity
IRootERC721Predicate.instance.depositBatch(rootToken, receivers, tokenIds);
```

### mapToken()

This function is used for token mapping.

#### Parameters

`rootToken` (IERC721Metadata): The address of the root token to map.

#### Usage

The `mapToken()` function is called internally on deposit if the token is not mapped already.
