The `IRootERC1155Predicate` interface is designed to work with ERC-1155 tokens on a rootchain. It provides functionality for depositing, withdrawing, and mapping ERC-1155 tokens between root and childchains. This user guide will explain how to interact with the functions provided by the `IRootERC1155Predicate` interface.

## Functions

### deposit()

This function deposits tokens from the depositor to themselves on the childchain.

#### Parameters

- `rootToken` (IERC1155MetadataURI): The root token being deposited.
- `tokenId` (uint256): The index of the NFT to deposit.
- `amount` (uint256): The amount to deposit.

#### Usage

To deposit tokens from the depositor to themselves on the childchain, call the `deposit()` function with the required parameters:

```solidity
IRootERC1155Predicate.instance.deposit(rootToken, tokenId, amount);
```

### depositTo()

This function deposits tokens from the depositor to another address on the childchain.

#### Parameters

- `rootToken` (IERC1155MetadataURI): The root token being deposited.
- `receiver` (address): The address of the receiver on the childchain.
- `tokenId` (uint256): The index of the NFT to deposit.
- `amount` (uint256): The amount to deposit.

#### Usage

To deposit tokens from the depositor to another address on the childchain, call the `depositTo()` function with the required parameters:

```solidity
IRootERC1155Predicate.instance.depositTo(rootToken, receiver, tokenId, amount);
```

### depositBatch()

This function deposits tokens from the depositor to other addresses on the childchain.

#### Parameters

- `rootToken` (IERC1155MetadataURI): The root token being deposited.
- `receivers` (address[]): The addresses of the receivers on the childchain.
- `tokenIds` (uint256[]): The indices of the NFTs to deposit.
- `amounts` (uint256[]): The amounts to deposit.

#### Usage

To deposit tokens from the depositor to other addresses on the childchain, call the `depositBatch()` function with the required parameters:

```solidity
IRootERC1155Predicate.instance.depositBatch(rootToken, receivers, tokenIds, amounts);
```

### mapToken()

This function is used for token mapping.

#### Parameters

`rootToken` (IERC1155MetadataURI): The address of the root token to map.

#### Returns

`childToken` (address): The address of the mapped child token.

#### Usage

To map a token, call the mapToken() function with the required parameter:

```solidity
address childToken = IRootERC1155Predicate.instance.mapToken(rootToken);
```

This function is called internally on deposit if the token is not mapped already.
