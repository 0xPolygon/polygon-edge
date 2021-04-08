
# Polygon SDK

Polygon SDK is a modular and extensible framework for building Ethereum-compatible blockchain networks. 

This repository is the first implementation of Polygon SDK, written in Golang. Other implementations, written in other programming languages might be introduced in the future. If you would like to contribute to this or any future implementation, please reach out to [Polygon team](mailto:contact@polygon.technology).

To find out more about Polygon, visit the [official website](https://polygon.technology/).

WARNING: This is a work in progress so architectural changes may happen in the future. The code has not been audited yet, so please contact [Polygon team](mailto:contact@polygon.technology) if you would like to use it in production.

## Structure

-   api - Server confiuration
    -   http - HTTP server for peer management and debugging
    -   jsonrpc - RPC server, endpoints: [](https://eth.wiki/json-rpc/API)[https://eth.wiki/json-rpc/API](https://eth.wiki/json-rpc/API), not fully implemented yet)
-   blockchain - Chain information (read/write blocks)
    -   storage - Storage implementations
-   chain - Chain parameters (active forks, consensus engine, etc.)
    -   chains - Predefined chain configurations (mainnet, goerli, ibft)
-   command - CLI commands
-   consensus - Consensus interface
    -   Clique - Clique engine (not fully implemented yet)
    -   Ethash - Ethash engine
    -   IBFT - IBFT engine
    -   POW - POW engine
-   crypto - Crypto utility functions
-   helper - Helper packages
    -   dao - Dao utils
    -   enode - Enode encoding/decoding function
    -   hex - Hex encoding/decoding functions
    -   ipc - IPC connection functions
    -   keccak - Keccak functions
    -   rlputil - Rlp encoding/decoding helper function
-   minimal - initialization (legacy name)
-   network - Start server and updates peer connections
    -   discovery - Discovers new peers and queues them
    -   transport - Peer communication transporting
-   protocol - Blockchain sync
-   scrips - Build script
-   sealer - Seals the block
-   state - State machine (currently implements EVM)
-   test - Test cases
-   types - Ethereum protocol types (block, transaction, etc..)
-   version - codebase version

## Dev

The easiest way to start with Polygon SDK is to "bypass" consensus and networking and start a blockchain locally. This is enabled with `dev` command that starts a local node and mines every transaction in a separate block. 

```
go run main.go dev
```

The current list of implemented RPC methods:
* [eth_blockNumber](https://eth.wiki/json-rpc/API)
* [eth_getBlockByNumber](https://eth.wiki/json-rpc/API)
* [eth_sendRawTransaction](https://eth.wiki/json-rpc/API)
* [eth_sendTransaction](https://eth.wiki/json-rpc/API)
* [eth_getTransactionReceipt](https://eth.wiki/json-rpc/API)
* [eth_getBalance](https://eth.wiki/json-rpc/API)
* [eth_getTransactionCount](https://eth.wiki/json-rpc/API)
* [eth_getCode](https://eth.wiki/json-rpc/API)
* [web3_sha](https://eth.wiki/json-rpc/API)

## Pluggable Consensus

Polygon SDK is designed to offer off-the-shelf pluggable consensus algortihms.

The current list of supported consensus algorithms:
1. IBFT
2. Ethereum's Nakamoto PoW
3. Clique PoA (not fully implemented yet)

We plan to add support for more consensus algorithms in the future (HotSuff, Tendermint etc). Contact us if you would like to use a specific, not yet supported algorithm for your project.

## Contributing

### Protobuf

We use version 3.12.0 of protobuf and version 1.1.0 of protoc-gen-go-grpc.

## Example

Start one node:

```
$ go run main.go server --data-dir ./test-chain-1 --grpc :10000 --libp2p :10001 --jsonrpc :10002 --seal
```

By default, the client uses an empty genesis file with a ~5s PoW.

In the logs of the running client you will see the LibP2P address required to connect to this node.

Start a second node that connects to the first:

```
$ go run main.go server --data-dir ./test-chain-2 --grpc :20000 --libp2p :20001 --jsonrpc :20002 --seal --join <node-1-libp2p-addr>
```

Monitor the blockchain events (i.e forks, reorgs...) in node 2.

```
$ go run main.go monitor --address localhost:20000
```

## Ibft

Init some data folders for ibft

```
$ go run main.go ibft init --data-dir test-chain-1
$ Done!
$ go run main.go ibft init --data-dir test-chain-2
$ Done!
$ go run main.go ibft init --data-dir test-chain-3
$ Done!
```

Generate an ibft genesis file with the previous accounts as validators

```
$ go run main.go genesis --ibft --ibft-validators-prefix-path test-chain-
```

Run all the clients.

```
$ go run main.go server --data-dir ./test-chain-1 --chain genesis.json --grpc :10000 --libp2p :10001 --jsonrpc :10002 --seal
```

```
$ go run main.go server --data-dir ./test-chain-2 --chain genesis.json --grpc :20000 --libp2p :20001 --jsonrpc :20002 --seal
```

```
$ go run main.go server --data-dir ./test-chain-3 --chain genesis.json --grpc :30000 --libp2p :30001 --jsonrpc :30002 --seal
```
