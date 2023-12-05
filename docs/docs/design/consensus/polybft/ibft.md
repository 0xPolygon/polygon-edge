## Consensus Engine

The PolyBFT consensus mechanism uses the IBFT 2.0 consensus engine to agree on adding new blocks to the blockchain. The validator pool in IBFT 2.0 is responsible for validating candidate blocks proposed by a randomly selected block proposer who is part of the validator pool. The proposer is responsible for constructing a block at the block interval. The proposer mechanism is based on [Tendermint](https://tendermint.com/), where a proposer is chosen based on a deterministic selection algorithm. The frequency of selection is proportional to the voting power of the validator.

Each block in IBFT 2.0 requires at least one round of voting by the validator to arrive at consensus, which is recorded as a collection of signatures on the block content. In general, a supermajority of validators must confirm that a block is valid for the block to be added to the blockchain. Only when there is no consensus on a given block, multiple rounds of voting are needed. The ideal path would be when the validator pool reaches consensus on a candidate block in the first round of voting, and the block is added to the blockchain without the need for additional rounds of voting. This is the most efficient and optimal outcome, as it allows the network to continue processing transactions and adding new blocks to the chain in a timely manner.

A validator's voting power is proportional to the amount of stake they have locked up on the network. This means that validators with more stake will have more voting power and, therefore, more influence over the decision-making process on the network. This also provides an economic incentive for validators to behave honestly and act in the network's best interest.

!!! info "Sealing a block"

    ### State transitions

    PolyBFT's consensus mechanism follows a series of state transitions that ensure network-wide consensus on the blockchain's state. The consensus process starts with a validator proposing a new block to be added to the blockchain, which includes a list of transactions to update the blockchain's state.

    Next, validators in the active set vote on whether to accept the proposed block. Each validator's voting weight determines their influence in voting, and a supermajority of validators must agree to accept the block for consensus to be reached. The protocol tracks the current block's position, also known as its **sequence**.

    The process to finalize a block in PolyBFT is referred to as **sealing**. When a validator proposes a new block, other validators vote on whether to accept it, and this process can be repeated multiple times. Each repetition is called a **round**, and during each round, a set number of validators must agree to seal the proposed block for it to be added to the blockchain. If the required number of votes isn't reached during a particular round, the voting process will continue into the next round, and the protocol "increases the round". Another validator will then attempt to seal the sequence in the new round.

    If the proposed block is accepted, it's added to the blockchain, and the state of the blockchain is updated to reflect the changes introduced by the transactions in the block. This process continues with the next proposer proposing a new block, and the process repeats.

### Validator Set

PolyBFT limits network participation to around 100 validators, and a variable amount of stake is used as a fixed stake criterion to limit the system's security and can make the system economically vulnerable. The validator set in the PolyBFT does not update on each block but is fixed during n block periods known as an epoch.

The `n` block period to define one epoch is determined by `genesis` configuration, and until then, validators will remain the same. At the end of the epoch, a special state transaction to validatorSetManagementContract is emitted, notifying the system about the validators' uptime during the epoch. It is up to the smart contract to reward validators by their uptime and update the validator set for the next epoch. There is a function getValidatorSet which returns the current validator set at any time.

!!! note
    Proposer selection algorithm - Section is being updated

The proposer selection algorithm is Tendermint-based.

### Staking

Staking is managed by staking contracts on the Edge-powered chain. The staking module on Polygon validates staked tokens and is independent of Ethereum's security. In principle, the network is secured by the rootchain and Ethereum. Transaction checkpoints still occur on Ethereum, but Ethereum does not validate staking on Polygon.

At the end of each epoch, a reward calculation occurs to reward validators who actively participated in that epoch.

!!! info
    Staking details and rewards - Section is being updated


## Benefits

### Consensus benefits

- **Immediate block finality**: Only one block is proposed at a given chain height; thus, the single chain removes forking, uncle blocks, and the risk that a transaction may be "undone" once on the chain later.
- **Reduced time between blocks**: The effort needed to construct and validate blocks is decreased significantly, which increases the chain's throughput.
- **High data integrity and fault tolerance**: IBFT 2.0 uses a pool of validators to ensure the integrity of each proposed block. A supermajority (~66%) of these validators must sign the block before insertion into the chain, making block forgery very difficult. Also, the proposer of the block rotates over time, ensuring a faulty node cannot exert long-term influence over the chain.
- **Operationally flexible**: The validators can be modified quickly, ensuring the group contains only fully trusted nodes.

### Network benefits

- **Liveness**: It has been proven that IBFT 2.0 does not guarantee BFT persistence nor liveness when operating on a synchronous network. If a validator receives enough confirmation about a block, it can lock the proposed block (assuming it has not locked any prior). If a change were to occur because of a fault in the network, it could trigger the activation of the round change protocol, where the protocol would expect to commit the locked block at that specific height.
- **Persistence**: If, for instance, there is a faulty network condition present where two different node subsets lock to two different blocks, the system enters into an infinite cycle of state transitions that cannot converge states and finalize the block.
