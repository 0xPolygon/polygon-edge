The `ICheckpointManager` interface is a crucial part of the checkpoint system, allowing validators to submit signed checkpoints as proof of the canonical chain. This guide will explain the methods the ICheckpointManager interface provides and how to use them.

## Overview

The `ICheckpointManager` interface is responsible for managing checkpoints in the system. It provides methods for submitting checkpoints, verifying event membership by block number or epoch, and retrieving checkpoint information.

## Submit a checkpoint

To submit a checkpoint, call the submit method:

```solidity
function submit(
    CheckpointMetadata calldata checkpointMetadata,
    Checkpoint calldata checkpoint,
    uint256[2] calldata signature,
    Validator[] calldata newValidatorSet,
    bytes calldata bitmap
) external;
```

The parameters for submitting a checkpoint include the following:

- The checkpoint metadata.
- The checkpoint itself.
- The aggregated signature.
- The new validator set.
- A bitmap of the old validator set that signed the message.
- The contract will verify the provided signature against the stored validator set.

## Get event membership by block number

To check if an event is part of the event root for a given block number, call the getEventMembershipByBlockNumber method:

```solidity
function getEventMembershipByBlockNumber(
    uint256 blockNumber,
    bytes32 leaf,
    uint256 leafIndex,
    bytes32[] calldata proof
) external view returns (bool);
```

This method takes the block number, the leaf of the event (keccak256-encoded log), the leaf index in the Merkle root tree, and the proof for leaf membership in the event root tree. It returns true if the event is part of the event root.

## Get event membership by epoch

To check if an event is part of the event root for a given epoch, call the getEventMembershipByEpoch method:

```solidity
function getEventMembershipByEpoch(
    uint256 epoch,
    bytes32 leaf,
    uint256 leafIndex,
    bytes32[] calldata proof
) external view returns (bool);
```

This method takes the epoch ID, the leaf of the event (keccak256-encoded log), the leaf index in the Merkle root tree, and the proof for leaf membership in the event root tree. It returns true if the event is part of the event root.

## Get checkpoint block number

To get the checkpoint block number for a given block number, call the getCheckpointBlock method:

```solidity
function getCheckpointBlock(uint256 blockNumber) external view returns (bool, uint256);
```

This method takes the block number and returns a boolean indicating if the block number was checkpointed, as well as the checkpoint block number.

## Get event root by block

To get the event root for a given block number, call the getEventRootByBlock method:

```solidity
function getEventRootByBlock(uint256 blockNumber) external view returns (bytes32);
```
