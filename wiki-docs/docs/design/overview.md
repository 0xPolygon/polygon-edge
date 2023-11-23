The diagram below illustrates the major components of Edge.

<div align="center">
  <img src="/img/edge/supernets-overview.excalidraw.png" alt="Edge architecture overview" width="850" />
</div>

## Components

The following table breaks down each of these components.

| Component | Description | Link |
| --- | --- | --- |
| libp2p | Provides the networking layer for Edge and is designed for peer-to-peer network architectures. | [<ins>libp2p Overview</ins>](libp2p.md) |
| Bridge | An in-built bridging mechanism enabled by PolyBFT that allows message passing between an Edge-powered chain and another Proof-of-Stake blockchain without mapping. | [<ins>Native Bridge overview</ins>](bridge/overview.md) |
| Mempool | Enables multiple validators to aggregate their signatures to create a single, aggregated signature representing all validators in the pool. | [<ins>mempool Overview</ins>](mempool.md) |
| Consensus | PolyBFT serves as the consensus mechanism of Polygon Edge and consists of a consensus engine, IBFT 2.0, and a consensus protocol that includes the bridge, staking, and other utilities. | [<ins>PolyBFT Overview</ins>](consensus/polybft/overview.md) |
| Blockchain | Coordinates everything in the system, curates state transitions, and is responsible for state changes when a new block is added to the chain. | [<ins>Blockchain Overview</ins>](blockchain.md) |
| Runtime (EVM) | Uses the EVM as the runtime environment for executing smart contracts. | [<ins>Runtime Overview</ins>](runtime/overview.md) |
| TxPool | Represents the transaction pool, closely linked with other modules in the system. | [<ins>TxPool Overview</ins>](txpool.md) |
| JSON-RPC | Facilitates interaction between dApp developers and the blockchain, allowing developers to issue JSON-RPC requests to an Edge node and receive responses. | [<ins>JSON-RPC Overview</ins>](jsonrpc.md) |
| gRPC | Essential for operator interactions, allowing node operators to interact with the client easily and providing a seamless user experience. | [<ins>gRPC Overview</ins>](grpc.md) |
