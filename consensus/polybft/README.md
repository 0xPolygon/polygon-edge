
# Polybft consensus protocol

Polybft is a consensus protocol, which runs [go-ibft](https://github.com/0xPolygon/go-ibft) consensus engine.  

It has native support for running bridge, which enables running cross-chain transactions with Ethereum-compatible blockchains.

## Setup local testing environment

### Precondition

1. Build binary

    ```bash
    go build -o polygon-edge .
    ```

2. Init secrets - this command is used to generate account secrets (ECDSA, BLS as well as P2P networking node id). `--data-dir` denotes folder prefix names and `--num` how many accounts need to be created. **This command is for testing purposes only.**

    ```bash
    polygon-edge polybft-secrets --data-dir test-chain- --num 4
    ```

3. Start rootchain server - rootchain server is a Geth instance running in dev mode, which simulates Ethereum network. **This command is for testing purposes only.**

    ```bash
    polygon-edge rootchain server
    ```

4. Generate manifest file - manifest file contains public validator information as well as bridge configuration. It is intermediary file, which is later used for genesis specification generation as well as rootchain contracts deployment.

    There are two ways to provide validators information:

    - all the validators information are present in local storage of single host and therefore directory if provided using `--validators-path` flag and validators folder prefix names using `--validators-prefix` flag

        ```bash
        polygon-edge manifest [--validators-path ./] [--validators-prefix test-chain-]
        [--path ./manifest.json] [--premine-validators 100]
        ```

    - validators information are scafollded on multiple hosts and therefore necessary information are supplied using `--validators` flag. Validator information needs to be supplied in the strictly following format:
    `<multi address>:<public ECDSA address>:<public BLS key>:<BLS signature>`.
    **Note:** when specifying validators via validators flag, entire multi address must be specified.

        ```bash
        polygon-edge manifest 
        --validators /ip4/127.0.0.1/tcp/30301/p2p/16Uiu2HAmV5hqAp77untfJRorxqKmyUxgaVn8YHFjBJm9gKMms3mr:0xDcBe0024206ec42b0Ef4214Ac7B71aeae1A11af0:1cf134e02c6b2afb2ceda50bf2c9a01da367ac48f7783ee6c55444e1cab418ec0f52837b90a4d8cf944814073fc6f2bd96f35366a3846a8393e3cb0b19197cde23e2b40c6401fa27ff7d0c36779d9d097d1393cab6fc1d332f92fb3df850b78703b2989d567d1344e219f0667a1863f52f7663092276770cf513f9704b5351c4:11b18bde524f4b02258a8d196b687f8d8e9490d536718666dc7babca14eccb631c238fb79aa2b44a5a4dceccad2dd797f537008dda185d952226a814c1acf7c2
        [--validators /ip4/127.0.0.1/tcp/30302/p2p/16Uiu2HAmGmidRQY5BGJPGVRF8p1pYFdfzuf1StHzXGLDizuxJxex:0x2da750eD4AE1D5A7F7c996Faec592F3d44060e90:088d92c25b5f278750534e8a902da604a1aa39b524b4511f5f47c3a386374ca3031b667beb424faef068a01cee3428a1bc8c1c8bab826f30a1ee03fbe90cb5f01abcf4abd7af3bbe83eaed6f82179b9cbdc417aad65d919b802d91c2e1aaefec27ba747158bc18a0556e39bfc9175c099dd77517a85731894bbea3d191a622bc:08dc3006352fdc01b331907fd3a68d4d68ed40329032598c1c0faa260421d66720965ace3ba29c6d6608ec1facdbf4624bca72df36c34afd4bdd753c4dfe049c]
        [--path ./manifest.json] [--premine-validators 100]
        ```

5. Deploy and initialize rootchain contracts - this command deploys rootchain smart contracts and initializes them. It also updates manifest configuration with rootchain contract addresses and rootchain default sender address.

    ```bash
    polygon-edge rootchain init-contracts 
    --data-dir <local_storage_secrets_path> | [--config <cloud_secrets_manager_config_path>] 
    [--manifest ./manifest.json]
    [--json-rpc http://127.0.0.1:8545]
    [--test]
    ```

6. Create chain configuration - this command creates chain configuration, which is needed to run a blockchain

    ```bash
    polygon-edge genesis --block-gas-limit 10000000 --epoch-size 10
    [--consensus polybft] [--bridge-json-rpc <rootchain_ip_address>] [--manifest ./manifest.json]
    ```

7. Fund validators on rootchain - in order for validators to be able to send transactions to Ethereum, they need to be funded in order to be able to cover gas cost. **This command is for testing purposes only.**

    ```bash
    polygon-edge rootchain fund --data-dir test-chain- --num 4
    ```

8. Run (child chain) cluster, consisting of 4 Edge clients in this particular example

    ```bash
    polygon-edge server --data-dir ./test-chain-1 --chain genesis.json --grpc-address :5001 --libp2p :30301 --jsonrpc :9545 --seal --log-level DEBUG

    polygon-edge server --data-dir ./test-chain-2 --chain genesis.json --grpc-address :5002 --libp2p :30302 --jsonrpc :10002 --seal --log-level DEBUG

    polygon-edge server --data-dir ./test-chain-3 --chain genesis.json --grpc-address :5003 --libp2p :30303 --jsonrpc :10003 --seal --log-level DEBUG
    
    polygon-edge server --data-dir ./test-chain-4 --chain genesis.json --grpc-address :5004 --libp2p :30304 --jsonrpc :10004 --seal --log-level DEBUG
    ```

    It is possible to run child chain nodes in "relayer" mode. It allows automatic execution of deposit events on behalf of users.
    In order to start node in relayer mode, it is necessary to supply `--relayer` flag:

    ```bash
    polygon-edge server --data-dir ./test-chain-1 --chain genesis.json --grpc-address :5001 --libp2p :30301 --jsonrpc :9545 --seal --log-level DEBUG --relayer
    ```
