## Overview

The ExitHelper contract is a smart contract that allows for the process of
exits, when a user wants to withdraw their assets from a `childchain` to a
`rootchain`. It uses a Merkle tree to check whether a state transition is included
in a block and provides functions to process exits and batch exits. The contract
also has an event trigger that is emitted when an exit is processed. The contract
is initialized with a [`CheckpointManager`](checkpoint-manager.md) address.

## Mappings

The following mappings are used in the contract:

- `processedExits`: a mapping from uint256 (exit id) to bool indicating whether an
  exit with a particular `id` has already been processed

The `processedExits` mapping is used to track which exits have already been processed and
prevent duplicate processing of the same exit.

## Events

### ExitProcessed

```js
event ExitProcessed(uint256 indexed id, bool indexed success, bytes returnData);
```

The contract includes an event called `ExitProcessed`, which is emitted when an exit
is processed. The event includes the following indexed fields:

- `id`: the identifier of the exit
- `success`: the result of processing the exit
- `returnData`: return data from the onL2StateReceive function

## Functions

### initialize

```js
function initialize(ICheckpointManager newCheckpointManager) external initializer {
    require(
        address(newCheckpointManager) != address(0) && address(newCheckpointManager).code.length != 0,
        "ExitHelper: INVALID_ADDRESS"
    );
    checkpointManager = newCheckpointManager;
}
```

This function is used to initialize the contract with the address of the
[CheckpointManager](checkpoint-manager.md) contract. It takes the following parameter:

- `newCheckpointManager`: the address of the checkpoint manager contract

The function requires that the address of the checkpoint manager contract is not the
zero address and that the contract has deployed code. If either of these checks fail,
the contract will revert with an appropriate error message.

### exit

```js
function exit(
    uint256 blockNumber,
    uint256 leafIndex,
    bytes calldata unhashedLeaf,
    bytes32[] calldata proof
) external onlyInitialized {
    // Implementation details ...
}
```

This function allows anyone to initiate an exit by calling the function
and passing in the block `number`, leaf `index`, unhashed `leaf`, and `proof`.
The function will check that the contract has been initialized by calling the
`onlyInitialized` modifier, which checks that the `checkpointManager` address
is not the zero address.

The function will then call the `_exit` function and pass the parameters to it.

### batchExit

```js
function batchExit(BatchExitInput[] calldata inputs) external onlyInitialized{
    // Implementation details ...
}
```

This function allows anyone to initiate multiple exits at once by calling the
function and passing in an array of `BatchExitInput` structs. The function will
check that the contract has been initialized by calling the `onlyInitialized` modifier,
which checks that the `checkpointManager` address is not the zero address.

The function will then loop through the inputs array, calling the `_exit` function for
each input.

### _exit

```js
function _exit(
    uint256 blockNumber,
    uint256 leafIndex,
    bytes calldata unhashedLeaf,
    bytes32[] calldata proof,
    bool isBatch
) private{
   // Implementation details ...
}
```

This function is the internal function that handles the actual execution of an `exit`.
It takes the following parameters:

- `blockNumber`: the block number of the state
- `leafIndex`: the leaf index of the state in the Merkle tree
- `unhashedLeaf`: the unhashed leaf of the state
- `proof`: the Merkle proof of the state
- `isBatch`: a boolean indicating whether the exit is part of a batch or not

This function first checks the `processedExits` mapping to make sure that the `exit`
with the given `id` has not been processed before.

Then it calls the checkpointManager's `getEventMembershipByBlockNumber` function and
passing `blockNumber`, `keccak256(unhashedLeaf)`, `leafIndex`, `proof` as arguments,
this function verifies the proof of inclusion of the unhashed leaf in the Merkle tree
of the given block number. If the proof is valid, it marks the exit as processed and
calls the `onL2StateReceive` function of the receiver contract with the `id`, `sender`
and data of the `exit` and emits the `ExitProcessed` event to indicate whether the call
was successful or not.

The `isBatch` parameter is used to determine if this is a single exit or part of a batch
of exits, if it's a batch, the function will not revert and just return if the exit has
been processed before.
