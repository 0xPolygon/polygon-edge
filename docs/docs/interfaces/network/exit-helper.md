The `IExitHelper` interface is a helper contract that processes exits from stored event roots in `CheckpointManager`. It allows users to perform an exit for one or multiple events. This user guide will explain how to interact with the functions provided by the IExitHelper interface.

## Functions

### exit()

This function allows you to perform an exit for one event.

#### Parameters

- `blockNumber` (uint256): The block number of the exit event on L2.
- `leafIndex` (uint256): The leaf index in the exit event Merkle tree.
- `unhashedLeaf` (bytes): The ABI-encoded exit event leaf.
- `proof` (bytes32[]): The proof of the event inclusion in the tree.

#### Usage

To exit an event, call the `exit()` function with the required parameters:

```solidity
IExitHelper.exitHelperInstance.exit(blockNumber, leafIndex, unhashedLeaf, proof);
```

### batchExit()

This function allows you to perform a batch exit for multiple events.

#### Parameters

- `inputs` (BatchExitInput[]): An array of BatchExitInput structs, where each struct contains the following fields:
    - `blockNumber` (uint256): The block number of the exit event on L2.
    - `leafIndex` (uint256): The leaf index in the exit event Merkle tree.
    - `unhashedLeaf` (bytes): The ABI-encoded exit event leaf.
    - `proof` (bytes32[]): The proof of the event inclusion in the tree.

#### Usage

To perform a batch exit for multiple events, create an array of `BatchExitInput` structs and call the `batchExit()` function:

```solidity
let batchExitInputs = [
  {
    blockNumber: blockNumber1,
    leafIndex: leafIndex1,
    unhashedLeaf: unhashedLeaf1,
    proof: proof1
  },
  {
    blockNumber: blockNumber2,
    leafIndex: leafIndex2,
    unhashedLeaf: unhashedLeaf2,
    proof: proof2
  }
];

IExitHelper.exitHelperInstance.batchExit(batchExitInputs);
```

This will process exits for multiple events in a single transaction, which can save gas and improve efficiency.
