## Introduction

State synchronization (StateSync) is a mechanism used to update the state of a contract on the Edge-powered chain based on events occurring on the rootchain. It is a critical component of blockchain technology as it enables secure and efficient communication between the two chains. State synchronization allows for a more efficient and secure way to update the chain state on the Edge-powered chain without needing to process all transactions from the genesis block.

## StateSync in Edge

!!! info "Key points"

    - `StateSync` enables the efficient and secure transfer of data between an Edge-powered chain and rootchain.
    - `StateSync` is initiated on the Edge-powered chain through the `StateSender` contract and executed on the rootchain through the `StateReceiver` contract.
    - The `StateSync` process is used to update the state of a contract on the rootchain based on events occurring on the Edge-powered chain.

### StateSender

The `StateSender` contract is deployed on the rootchain and is triggered by either the associated rootchain predicate or `SupernetManager` rootchain contract. Its main responsibility is to generate sync state events based on the provided data and receiver address. Anyone can call the `syncState` function to emit a sync state event. The data is sent along with the event and represents the state change that needs to be executed on the Edge-powered chain.

### StateReceiver

The `StateReceiver` contract is deployed on the Edge and is responsible for executing and relaying the state data sent from the rootchain. It receives the state change data from the rootchain contract bundled up in the form of a commitment, sent with the Merkle Tree root hash. This tree is created by bundling a number of `StateSync` events received by the `StateSender`. Commitments are submitted to the `StateReceiver` by a block proposer, and it is a system (state) transaction. They are used to verify the execution of state data from the rootchain to the Edge-powered chain, such as transferring funds from rootchain to Edge. Commitments are similar to checkpoints but are used in the process of transferring data from rootchain to Edge, while checkpoints are used in the process of transferring data from Edge to rootchain.

## L2StateSender and ExitHelper

To enable communication from the Edge-powered chain to the rootchain, the `L2StateSender` contract resides on an Edge-powered chain and is responsible for emitting `L2StateSyncs` (also referred to as exit events). These events are indexed by the rootchain validators and submitted as a checkpoint on the rootchain, allowing for lazy execution. Unlike the `StateSender`, there is no transaction execution on the rootchain for the `L2StateSender`.

On the rootchain, the `ExitHelper` contract is responsible for verifying the execution of the exit events and enabling users to withdraw their funds from the Edge-powered chain to the rootchain. It is analogous to the StateReceiver on the Edge-powered chain, and both contracts work together to enable two-way communication between the rootchain and the Edge-powered chain.

!!! info "Synchronization and commitments"

    The `StateSync` process involves two main steps: synchronization and commitments.

    In the synchronization step, the `StateSender` contract on the rootchain generates sync state events based on receiver and data. The `syncState` function allows anyone to call this method to emit an event. The data is sent along with the event and represents the state change that needs to be executed on the Edge.

    In the commitments step, the `StateReceiver` contract on the Edge-powered chain receives the state change data along with a Merkle proof from the `StateSender` contract and verifies the proof to ensure the data's integrity. If the proof is valid, the state change is executed on the Edge-powered chain.

    To ensure the validity of the state change, the `StateSender` contract generates a unique id for each sync state event. This id is used by the `StateReceiver` contract to prevent replay attacks, which could result in the execution of duplicate state changes.

    The `StateReceiver` contract also implements a BLS signature scheme to verify the signatures submitted by the validators. The validators' signatures are aggregated, and the contract checks whether the required voting power threshold is met to accept the state change.
