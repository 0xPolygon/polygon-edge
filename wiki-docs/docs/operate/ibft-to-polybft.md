## Transitioning to a New Blockchain with PolyBFT Consensus from a Blockchain using IBFT Consensus with Edge PoA

In this guide, you'll discover how to migrate from an existing IBFT consensus chain on the former Edge client to a new PolyBFT consensus-based chain.

!!! caution "This is not an upgrade"

    The regenesis process is not intended as an upgrade to another version of the consensus client but rather as a transformation to a distinct product suite designed for next-generation, application-specific chains (appchains) with cross-chain compatibility and staking requirements. If you are unfamiliar or uncertain about this, please refer to the [<ins>system design documentation</ins>](/design/overview). For further assistance, please reach out to the Polygon team.

!!! info "Compatibility"

    The regenesis process has ONLY been tested for version(s) 0.6.x and not with earlier versions of the former Edge client. The team is actively working on testing compatibility with older versions. For assistance, please reach out to the Polygon team.

    In general, it is recommended that you operate on the latest version of the old client. The latest version of the former Edge consensus client is [<ins>v0.6.3</ins>](https://github.com/0xPolygon/polygon-edge/releases/tag/v0.6.3).

    To upgrade Edge, please refer to the archived Edge documentation, available **[<ins>here</ins>](https://github.com/0xPolygon/wiki/tree/master/archive/edge/main-edge)**.

## Overview

The regenesis process focuses on the migration of the state trie, also known as the state tree, from an existing chain to a new chain. The state trie is a Merkle Patricia trie that maintains the entire state of the blockchain, including account balances, contract storage, and other relevant data. Regenesis also involves processing block headers and validating state roots to ensure the correctness and consistency of the migrated state trie. However, regenesis does not involve the migration of transaction history, as it is specifically concerned with state trie migration and block header validation.

## What you'll learn

In this guide, you'll gain an understanding of the following concepts:

- Migrating from an existing IBFT consensus chain to a new chain using the PolyBFT consensus algorithm.
- Generating a trie snapshot, setting up new validators, and creating the chain configuration for the new chain.
- Launching a new PolyBFT chain and ensuring the successful transfer of the snapshot trie to each validator node.

## What you'll do

- **State Trie Migration**: The process starts with obtaining the trie root of the existing chain. A snapshot of the trie is created using this root, which will be utilized in the new chain.

- **Chain Setup**: The old chain data is removed, and new validators are created for the new chain. The genesis file is generated for the new chain, using the trie root from the snapshot.

- **Starting the New Chain**: The new PolyBFT chain is started, and the snapshot trie is copied to each validator node's data directory. This process ensures that the new chain's state is consistent with the old chain's state.

- **Resuming the Chain**: The new chain is restarted, and the validator nodes continue to seal new blocks. The state trie from the previous chain has been successfully migrated, allowing the new chain to maintain the same account balances, contract storage, and other state data.

- **Verification**: After the regenesis process is complete, you can verify the success of the migration by checking the account balances on the new chain. If the account balance is non-zero and matches the previous chain's state, the snapshot import was successful.

Before beginning the regenesis process, there are some prerequisites that must be fulfilled.

## Prerequisites

- Set up a local IBFT consensus cluster for development purposes.

  A dedicated script is provided as part of the client to facilitate the cluster setup, encompassing key generation, network configuration, data directory creation, and genesis block generation. To create an IBFT cluster, execute the following command:

  ```bash
  scripts/cluster ibft
  ```

- Ensure that the accounts have sufficient funds.

  Before performing the regenesis, it's essential to ensure that the accounts have sufficient funds. You can check the balance using the following command:

  ```bash
  curl -s -X POST --data '{"jsonrpc":"2.0", "method":"eth_getBalance", "params":["0x85da99c8a7c2c95964c8efd687e95e632fc533d6", "latest"], "id":1}' http://localhost:10002
  ```

!!! caution "Don't use the develop branch for deployments"
    Please ensure that you are not running on the `develop` branch, which is the active development branch and include changes that are still being tested and not compatible with the current process.

    Instead, use the [<ins>latest release</ins>](/operate/install) for deployments.

## Regenesis

The regenesis process involves several steps, which are outlined below.

!!! note "Configuration Parameters"

    For a list of configurable parameters, please refer to the reference guide available [<ins>here</ins>](/operate/param-reference).

### 1. Get trie root

Obtain the trie root of the existing chain, which is required to create a snapshot of the state trie.

```bash
./polygon-edge regenesis getroot --rpc "http://localhost:10002"
```

> `10002` is the port number that is specified for the HTTP-RPC server in the configuration file for the polygon-edge node. It is the default port number used by the polygon-edge client for the HTTP-RPC server.

### 2. Create trie snapshot*

Create a snapshot of the state trie using the trie root. This snapshot contains the state data, including account balances and contract storage. The `stateRoot` is the root hash of the Merkle trie data structure that represents the current state of the blockchain at a given block height. It can be obtained by querying the blockchain node through its JSON-RPC API.

```bash
./polygon-edge regenesis \
  --target-path ./trie_new \
  --stateRoot 0xf5ef1a28c82226effb90f4465180ec3469226747818579673f4be929f1cd8663 \
  --source-path ./test-chain-1/trie
```

### 3. Remove old chain data

Delete the data of the old chain in preparation for setting up the new chain.

```bash
rm -rf test-chain-*
```

### 4. Create new validators

Generate new validators for the new chain, ensuring that the chain operates with a fresh set of validator nodes.

> Please note that the `--insecure` flag used in the command is for testing purposes only and should not be used in production environments. Instead, secure methods should be used to store and pass secrets.

```bash
./polygon-edge polybft-secrets --insecure --data-dir test-chain- --num 4
```

### 5. Generate the genesis file

Generate the genesis file for the new chain, using the trie root from the snapshot. This file is essential for starting the new chain with the migrated state data.

```bash
./polygon-edge genesis --consensus polybft --bridge-json-rpc http://127.0.0.1:8545 \
  --block-gas-limit 10000000 \
  --epoch-size 10 \
  --trieroot 0xf5ef1a28c82226effb90f4465180ec3469226747818579673f4be929f1cd8663
```

### 6. Start your new PolyBFT chain

Launch the new PolyBFT chain with the copied snapshot trie, ensuring that the new chain's state is consistent with the old chain's state.

```bash
./polygon-edge server --data-dir ./test-chain-1 --chain genesis.json --grpc-address :10000 --libp2p :30301 --jsonrpc :10002 --log-level DEBUG &
./polygon-edge server --data-dir ./test-chain-2 --chain genesis.json --grpc-address :20000 --libp2p :30302 --jsonrpc :20002 --log-level DEBUG &
./polygon-edge server --data-dir ./test-chain-3 --chain genesis.json --grpc-address :30000 --libp2p :30303 --jsonrpc :30002 --log-level DEBUG &
./polygon-edge server --data-dir ./test-chain-4 --chain genesis.json --grpc-address :40000 --libp2p :30304 --jsonrpc :40002 --log-level DEBUG &
```

### 7. Copy the snapshot trie to the data directory

Copy the snapshot trie to the data directories of each validator node, allowing the new chain to maintain the same account balances, contract storage, and other state data.

```bash
rm -rf ./test-chain-1/trie
rm -rf ./test-chain-2/trie
rm -rf ./test-chain-3/trie
rm -rf ./test-chain-4/trie
cp -fR ./trie_new/ ./test-chain-1/trie/
cp -fR ./trie_new/ ./test-chain-2/trie/
cp -fR ./trie_new/ ./test-chain-3/trie/
cp -fR ./trie_new/ ./test-chain-4/trie/
```

### 8. Re-run the chain

Restart the new chain, and the validator nodes will continue to seal new blocks. The state trie from the previous chain has been successfully migrated.

```bash
./polygon-edge server --data-dir ./test-chain-1 --chain genesis.json --grpc-address :10000 --libp2p :30301 --jsonrpc :10002 --seal --log-level DEBUG &
./polygon-edge server --data-dir ./test-chain-2 --chain genesis.json --grpc-address :20000 --libp2p :30302 --jsonrpc :20002 --seal --log-level DEBUG &
./polygon-edge server --data-dir ./test-chain-3 --chain genesis.json --grpc-address :30000 --libp2p :30303 --jsonrpc :30002 --seal --log-level DEBUG &
./polygon-edge server --data-dir ./test-chain-4 --chain genesis.json --grpc-address :40000 --libp2p :30304 --jsonrpc :40002 --seal --log-level DEBUG &
```

### 9. Check account balance on the new PolyBFT chain

Verify the success of the migration by checking the account balances on the new chain. If the account balance is non-zero and matches the previous chain's state, the snapshot import was successful.

```bash
curl -s -X POST --data '{"jsonrpc":"2.0", "method":"eth_getBalance", "params":["0x85da99c8a7c2c95964c8efd687e95e632fc533d6", "latest"], "id":1}' http://localhost:10002

{"jsonrpc":"2.0","id":1,"result":"0x3635c9adc5dea00000"}%
```