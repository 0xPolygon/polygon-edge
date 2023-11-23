## Overview

The CheckpointManager contract is used by validators to submit signed
checkpoints as proof of the canonical chain. The contract verifies that the
provided signature is valid and that the checkpoint has been signed as expected.
The contract also includes an initialization function that can only be
called once to set the contract's dependencies and domain.

## Functions

### initialize

```js
function initialize(
    IBLS newBls,
    IBN256G2 newBn256G2,
    bytes32 newDomain,
    uint256 chainId_,
    Validator[] calldata newValidatorSet
 ) external initializer {
    // Implementation details ...
}
```

This function is the initializer for the CheckpointManager contract.
It sets the following contract dependencies and parameters:

- `newBls`: The address of the BLS library contract.
- `newBn256G2`: The address of the BN256G2 library contract.
- `newDomain`: The domain to use when hashing messages to a point.
- `chainId_`: The chain ID of the Ethereum network.
- `newValidatorSet`: The array of validators to seed the contract with.
  This is passed as calldata so that the contract doesn't have to be initialized
  with a static length.

### submit

```js
  function submit(
    CheckpointMetadata calldata checkpointMetadata,
    Checkpoint calldata checkpoint,
    uint256[2] calldata signature,
    Validator[] calldata newValidatorSet,
    bytes calldata bitmap
  ) external {
      // Implementation details ...
}
```

This function is used by validators to submit a new checkpoint.

- `checkpointMetadata`: Contains metadata about the submitted checkpoint,
  such as the current validator set hash and block hash.
- `checkpoint`: The checkpoint data, including the block number, epoch, event
  root, and nonce.
- `signature`: The signature from a group of validators, verifying the submitted
  checkpoint data.
- `newValidatorSet`: An array of updated validators for the contract.
- `bitmap`: The bitmap of the signatures, indicating which validators signed the
  checkpoint.

### getEventMembershipByBlockNumber

```js
function getEventMembershipByBlockNumber(
      uint256 blockNumber,
      bytes32 leaf,
      uint256 leafIndex,
      bytes32[] calldata proof
  ) external view returns (bool) {
    // Implementation details ...
  }
```

This function returns a boolean indicating whether the specified leaf is
a member of the event root corresponding to the given blockNumber.

- `blockNumber`: The block number to retrieve the event root for.
- `leaf`: The leaf to check membership for.
- `leafIndex`: The index of the leaf in the Merkle tree.
- `proof`: The Merkle proof demonstrating membership of the leaf in the
  Merkle tree.

### getEventMembershipByEpoch

```js
function getEventMembershipByEpoch(
      uint256 epoch,
      bytes32 leaf,
      uint256 leafIndex,
      bytes32[] calldata proof
  ) external view returns (bool) {
    // Implementation details ...
  }
```

This function returns the membership status of an event (specified by
`leaf` and `leafIndex`) in an event merkle tree (specified by `eventRoot`)
during a certain epoch of the Ethereum network.

- `epoch`: The epoch for which to check event membership.
- `leaf`: The 32-byte hash of the event.
- `leafIndex`: The position of the event in the event merkle tree.
- `proof`: The merkle proof to validate the membership of the event in the
  tree.
- The function returns true if the event is a member of the tree, and false
  otherwise.

### getEventRootByBlock

```js
  function getEventRootByBlock(uint256 blockNumber) external view returns (bytes32 root) {
      // Implementation details ...
  }
```

This function returns the Merkle root of the event log for a specific block number.

- `blockNumber`: The number of the block for which to retrieve the checkpoint Merkle
  tree root.
- `root`: The root of the Merkle tree of checkpoints for the specified block.

### _setNewValidatorSet

:::caution Current validator set

To bootstrap the network, the root validator is composed of internal
validators of Polygon Labs.

:::

```js
function _setNewValidatorSet(Validator[] calldata newValidatorSet) private {
    // Implementation details ...
  }
```

This is an internal function used to set a new validator set for the contract.

### _verifySignature

```js
function _verifySignature(
        uint256[2] memory message,
        uint256[2] calldata signature,
        bytes calldata bitmap
    ) private view {
    // Implementation details ...
  }
```

This function checks the validity of a checkpoint by verifying the signature
included in the checkpoint against a computed aggregate public key of the validators
and a message (both passed as inputs).

- `message`: A 2-element array representing the message to verify.
- `signature`: A 2-element array representing the signature to verify against the
  message and aggregate public key.
- `bitmap`: A byte array representing the bitmap indicating which validators were
  involved in signing the message.

- The function performs the following checks and throws an error if any check fails:
  - Bitmap is not empty
  - Total voting power represented by validators in the bitmap is greater than 2/3 of
    the total voting power of all validators
  - The signature is successfully verified against the message and aggregate public key.

### verifyCheckpoint

```js
function _verifyCheckpoint(uint256 prevId, Checkpoint calldata checkpoint) private view {
    Checkpoint memory oldCheckpoint = checkpoints[prevId];
    require(
    // Implementation details ...
    );
}
```

This function verifies a checkpoint by checking its validity.

- `prevId`: The ID of the current checkpoint.
- `checkpoint`: The checkpoint to be verified.

The function checks if the checkpoint has a valid epoch value, either equal to the
epoch value of the current checkpoint or one greater than it. It also checks if the
`blockNumber` of the checkpoint is greater than the `blockNumber` of the current checkpoint.
If both checks pass, the function returns true, otherwise it throws an error.

### _getValueFromBitmap

```js
function _getValueFromBitmap(bytes calldata bitmap, uint256 index) private pure returns (bool) {
    // Implementation details ...
}
```

This is an internal function which takes a `bitmap` and an `index` as
inputs and returns a boolean value. It works as follows:

- `byteNumber` is calculated as index / 8
- `bitNumber` is calculated as index % 8
- If `byteNumber` is greater than or equal to the length of `bitmap`, it returns false.
- It returns the result of the expression uint8(bitmap[byteNumber]) & (1 << `bitNumber`) > 0,
  which checks if the bitNumber-th bit of the byteNumber-th byte in bitmap is set to 1.
