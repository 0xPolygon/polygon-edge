
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

The easiest way to start with Polygon SDK is to "bypass" consensus and networking and start a blockchain locally. This is enabled with **dev** command that starts a local node and mines every transaction in a separate block. 

```
go run main.go dev
```

Use curl command to send transaction ([](https://eth.wiki/json-rpc/API)[https://eth.wiki/json-rpc/API](https://eth.wiki/json-rpc/API) - eth_sendTransaction method). Wait for the transaction to get mined, use eth_blockNumber and eth_getBlockByNumber methods.

In order to better understand each step during the block sealing (sealer.go), we suggest using a debugger.

## Pluggable Consensus

Polygon SDK is designed to offer off-the-shelf pluggable consensus algortihms.

The current list of supported consensus algorithms:
1. IBFT
2. Ethereum's Nakamoto PoW
3. Clique PoA (not fully implemented yet)

We plan to add support for more consensus algorithms in the future (HotSuff, Tendermint etc). Contact us if you would like to use a specific, not yet supported algorithm for your project.

### IBFT

Perform the following steps to activate networking and the IBFT consensus engine:
1. Generate genesis block that will contain a validator list:

```
go run main.go ibft-genesis [privateKey1, port1 privateKey2, port2 ...]
```
2. For each validator create data folder and insert `privateKey` in the file called `key`.

3. Start each validator:

```
go run main.go agent ibft --data-dir [folder] --port [port] --addr [address] --rpc-addr [rpcAddress] --rpc-port [rpcPort] --seal --log-level TRACE
```


