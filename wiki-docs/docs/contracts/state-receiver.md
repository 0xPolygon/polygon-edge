## Overview

The StateReceiver contract is a smart contract that allows for the execution
and relay of state data on a child blockchain. It uses a Merkle tree to commit
to a batch of state sync objects and provides functions to verify the inclusion
of individual state sync objects in a commitment. It also allows for the execution
of multiple state sync events at once and has a mapping to keep track of processed
state sync events. The contract also has several event triggers that are emitted
when specific actions are taken, such as when a new commitment is added, when a
state sync event is executed, and when the execution of a state sync event fails.

## Structs

### StateSync

```js
struct StateSync {
    uint256 id;
    address sender;
    address receiver;
    bytes data;
}
```

The `StateSync` struct is used to represent a single state sync event.
It includes the following fields:

- `id`: a unique identifier for the state sync event
- `sender`: the address of the sender of the state sync event
- `receiver`: the address of the receiver of the state sync event
- `data`: the data associated with the state sync event

### StateSyncCommitment

```js
struct StateSyncCommitment {
        uint256 startId;
        uint256 endId;
        bytes32 root;
    }
```

The `StateSyncCommitment` struct is used to represent a group of state
sync events that are processed together. It includes the following fields:

- `startId`: the identifier of the first state sync event in the bundle
- `endId`: the identifier of the last state sync event in the bundle
- `root`: the root of the Merkle tree formed by the state sync events in
- the bundle

## Mappings

The following mappings are used in the contract:

- `processedStateSyncs`: a mapping from uint256 (state sync id) to bool indicating
whether a state sync event with a particular id has already been processed
- `commitments`: a mapping from uint256 (commitment id) to StateSyncCommitment struct
  indicating the commitment associated with a particular state sync id
- `commitmentIds`: an array of uint256 representing the ids of all commitments in the
  contract

The `processedStateSyncs` mapping is used to track which state sync events have already
been executed and prevent duplicate execution of the same event. The `commitments` mapping
is used to store `commitments` and retrieve them later by state sync id. `commitmentIds` is
used to keep track of the order of `commitments` in the contract.

## Constants

```js
uint256 private constant MAX_GAS = 300000;
```

The contract defines a constant called `MAX_GAS`, which represents the maximum
gas allowed for each message call.

## Events

### StateSyncResult

```js
event StateSyncResult(uint256 indexed counter, ResultStatus indexed status, bytes32 message);
```

The contract includes an event called `StateSyncResult`, which is emitted when a `StateSync`
event is processed. The event includes the following indexed fields:

- `counter`: the identifier of the state sync event
- `status`: the result of processing the state sync event (success, failure, or skip)
- `message`: a message associated with the result of processing the state sync event

### NewCommitment

```js
event NewCommitment(uint256 indexed startId, uint256 indexed endId, bytes32 root);
```

The contract also includes an event called `NewCommitment`, which is emitted when a new
commitment is made. The event includes the following indexed fields:

- `startId`: the startId of the commitment
- `endId`: the endId of the commitment
- `root`: the root of the Merkle tree of the commitment

## Functions

### commit

```js
function commit(
    StateSyncCommitment calldata commitment,
    bytes calldata signature,
    bytes calldata bitmap
) external onlySystemCall
```

This function is used to commit the root of a Merkle tree formed by a group of
state sync events. It takes the following parameters:

- `commitment`: a StateSyncCommitment struct representing the group of state sync events
  to be committed
- `signature`: the signature of the commitment
- `bitmap`: a bitmap indicating which validators signed the commitment

The function checks that the `startId` of the `commitment` is the `lastCommittedId` plus 1,
and that the `endId` of the `commitment` is greater than or equal to the `startId`. It then
verifies the `signature` of the `commitment` using an aggregation of public keys.
If the `signature` is valid, it stores the `commitment` in the `commitments` mapping
and updates the `lastCommittedId`.

### execute

```js
function execute(bytes32[] calldata proof, StateSync[] calldata objs) external
```

This function is used to submit a leaf of a Merkle tree formed by a group of state
sync events for execution. It takes the following parameters:

- `proof`: an array of Merkle proofs for the state sync events in the group
- `objs`: an array of StateSync structs representing the state sync events in the group

The function first checks that there is a `bundle` in the `bundles` mapping that has not
yet been executed. It then calculates the data hash of the objs array and verifies that the
data hash is a leaf in the Merkle tree rooted at the root of the corresponding `StateSyncBundle`
struct. If the data hash is a valid leaf, the function increments the counter and updates the
`currentLeafIndex` and `lastExecutedBundleCounter` as necessary. Finally, it processes each state
sync event in the objs array by calling the processSync function for each event.

### batchExecute

```js
function batchExecute(bytes32[][] calldata proofs, StateSync[] calldata objs) external
```

This function is similar to the execute function but allows for multiple state sync events
to be executed at once. It takes the following parameters:

- `proofs`: an array of arrays of Merkle proofs for the state sync events in the group
- `objs`: an array of StateSync structs representing the state sync events in the group

The function first checks that the length of `proofs` is equal to the length of `objs`.
It then checks that there is a commitment in the commitments mapping that has not yet been
executed for each element in `objs`. It then calculates the data hash of each `obj` and
verifies that the data hash is a leaf in the Merkle tree rooted at the root of the
corresponding commitment. If the proof is valid, it executes the corresponding state
sync event.

### getRootByStateSyncId

```js
function getRootByStateSyncId(uint256 id) external view returns (bytes32)
```

This function allows for retrieval of the root data for a specific state sync event.
It takes the following parameter:

- `id`: the id of the state sync event

It first retrieves the root from the commitment with the highest `id` lower than or
equal to the provided `id`. It then checks if the root is not equal to `0` and returns
it if it is not, otherwise it throws an error.

### getCommitmentByStateSyncId

```js
function getCommitmentByStateSyncId(uint256 id) public view returns (StateSyncCommitment memory)
```

This function allows to retrieve the commitment for a state sync `id`.

It takes the following parameter:

- `id`: the state sync id to get the commitment for

The function first retrieves the index of the commitment in `commitmentIds`
using the `findUpperBound` function, passing the given `id`. It then checks if the index is
not equal to the length of `commitmentIds`, if not, it returns the commitment from
the `commitments` mapping using the calculated `index`. If it is, it will throw an error
`StateReceiver: NO_COMMITMENT_FOR_ID`

### _executeStateSync

```js
function _executeStateSync(StateSync calldata obj) private
```

This function is an internal function to execute a state sync object.

It takes the following parameter:

- `obj`: the `StateSync` object to be executed

The function first checks if the state sync has already been processed
by checking the `processedStateSyncs` mapping.

If it has, it will throw an error `StateReceiver: STATE_SYNC_IS_PROCESSED`.
If not, it will check if the receiver address has no code, if so, it will emit
a `StateSyncResult` event with false status, otherwise, it will mark the state
sync as processed and call the receiver address with the onStateReceive function
and the obj `id`, sender and data as parameters. It will then emit a `StateSyncResult`
event with the success status and return data of the call.

### _checkPubkeyAggregation

```js
function _checkPubkeyAggregation(bytes32 message, bytes calldata signature, bytes calldata bitmap) internal view
```

This function verifies an aggregated BLS signature using BLS precompile.
It takes the following parameters:

- `message`: plaintext of signed message
- `signature`: the signed message
- `bitmap`: bitmap of which validators have signed

The function uses the V`ALIDATOR_PKCHECK_PRECOMPILE` address to call the
`staticcall` function with the `abi.encode` of the message, `signature` and
`bitmap` passed as parameters. It then decodes the return data to a bool and
checks if the call was successful and the signature is verified. If not it will
throw an error `SIGNATURE_VERIFICATION_FAILED`.
