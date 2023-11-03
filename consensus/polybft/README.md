
# Polybft consensus protocol

Polybft is a consensus protocol, which runs [go-ibft](https://github.com/0xPolygon/go-ibft) consensus engine.  

It has a native support for running bridge, which enables running cross-chain transactions with Ethereum-compatible blockchains.

## Setup local testing environment

1. Build binary

    ```bash
    $ go build -o polygon-edge .
    ```

2. Init secrets - this command is used to generate account secrets (ECDSA, BLS as well as P2P networking node id). `--data-dir` denotes folder prefix names and `--num` how many accounts need to be created. **This command is for testing purposes only.**

    ```bash
    $ polygon-edge polybft-secrets --data-dir test-chain- --num 4
    ```

3. Create chain configuration - this command creates chain configuration, which is needed to run a blockchain.
   It contains initial validator set as well and there are two ways to specify it:

   - all the validators information are present in local storage of single host and therefore directory if provided using `--validators-path` flag and validators folder prefix names using `--validators-prefix` flag
   - for reward distribution to work, user must define a reward wallet address with its balance. Wallet address is used to distribute reward tokens from that address to validators that signed blocks in that epoch.

    ```bash
    $ polygon-edge genesis --block-gas-limit 10000000 --epoch-size 10 \
        --proxy-contracts-admin 0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed \
        [--validators-path ./] [--validators-prefix test-chain-] \
        [--consensus polybft] \
        [--reward-wallet address:amount]
    ```

   - validators information are scafollded on multiple hosts and therefore necessary information are supplied using `--validators` flag. Validator information needs to be supplied in the strictly following format:
   `<multi address>:<public ECDSA address>:<public BLS key>:<BLS signature>`.
    **Note:** when specifying validators via validators flag, entire multi address must be specified.

    ```bash
    $ polygon-edge genesis --block-gas-limit 10000000 --epoch-size 10 \
        --proxy-contracts-admin 0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed \
        --validators /ip4/127.0.0.1/tcp/30301/p2p/16Uiu2HAmV5hqAp77untfJRorxqKmyUxgaVn8YHFjBJm9gKMms3mr:0xDcBe0024206ec42b0Ef4214Ac7B71aeae1A11af0:1cf134e02c6b2afb2ceda50bf2c9a01da367ac48f7783ee6c55444e1cab418ec0f52837b90a4d8cf944814073fc6f2bd96f35366a3846a8393e3cb0b19197cde23e2b40c6401fa27ff7d0c36779d9d097d1393cab6fc1d332f92fb3df850b78703b2989d567d1344e219f0667a1863f52f7663092276770cf513f9704b5351c4:11b18bde524f4b02258a8d196b687f8d8e9490d536718666dc7babca14eccb631c238fb79aa2b44a5a4dceccad2dd797f537008dda185d952226a814c1acf7c2
        [--validators /ip4/127.0.0.1/tcp/30302/p2p/16Uiu2HAmGmidRQY5BGJPGVRF8p1pYFdfzuf1StHzXGLDizuxJxex:0x2da750eD4AE1D5A7F7c996Faec592F3d44060e90:088d92c25b5f278750534e8a902da604a1aa39b524b4511f5f47c3a386374ca3031b667beb424faef068a01cee3428a1bc8c1c8bab826f30a1ee03fbe90cb5f01abcf4abd7af3bbe83eaed6f82179b9cbdc417aad65d919b802d91c2e1aaefec27ba747158bc18a0556e39bfc9175c099dd77517a85731894bbea3d191a622bc:08dc3006352fdc01b331907fd3a68d4d68ed40329032598c1c0faa260421d66720965ace3ba29c6d6608ec1facdbf4624bca72df36c34afd4bdd753c4dfe049c]
    ```

4. Start rootchain server - rootchain server is a Geth instance running in dev mode, which simulates Ethereum network. **This command is for testing purposes only.**

    ```bash
    $ polygon-edge rootchain server
    ```

5. Deploy StakeManager - if not already deployed to rootchain. Command has a test flag used only in testing purposes which would deploy a mock ERC20 token which would be used for staking. If not used for testing, stake-token flag should be provided:

    ```bash
    $ polygon-edge polybft stake-manager-deploy \
     --deployer-key <hex_encoded_rootchain_account_private_key> \
     --proxy-contracts-admin 0xaddressOfProxyContractsAdmin \
    [--genesis ./genesis.json] \
    [--json-rpc http://127.0.0.1:8545] \
    [--stake-token 0xaddressOfStakeToken] \
    [--test]
    ```

6. Deploy and initialize rootchain contracts - this command deploys rootchain smart contracts and initializes them. It also updates genesis configuration with rootchain contract addresses and rootchain default sender address.

    ```bash
    $ polygon-edge rootchain deploy \
    --deployer-key <hex_encoded_rootchain_account_private_key> \
    --stake-manager <address_of_stake_manager_contract> \
    --stake-token 0xaddressOfStakeToken \
    --proxy-contracts-admin 0xaddressOfProxyContractsAdmin \
    [--genesis ./genesis.json] \
    [--json-rpc http://127.0.0.1:8545] \
    [--test]
    ```

7. Fund validators on rootchain - in order for validators to be able to send transactions to Ethereum, they need to be funded in order to be able to cover gas cost. **This command is for testing purposes only.**

    ```bash
    $ polygon-edge rootchain fund \
        --addresses 0x1234567890123456789012345678901234567890 \
        --amounts 200000000000000000000
    ```

8. Whitelist validators on rootchain - in order for validators to be able to be registered on the SupernetManager contract on rootchain. Note that only deployer of SupernetManager contract (the one who run the deploy command) can whitelist validators on rootchain. He can use either its hex encoded private key, or data-dir flag if he has secerets initialized:

    ```bash
    $ polygon-edge polybft whitelist-validators --private-key <hex_encoded_rootchain_account_private_key_of_supernetManager_deployer> \
    --addresses <addresses_of_validators> --supernet-manager <address_of_SupernetManager_contract>
    ```

9.  Register validators on rootchain - each validator registers itself on SupernetManager. **This command is for testing purposes only.**

    ```bash
    $ polygon-edge polybft register-validator --data-dir ./test-chain-1 \
    --supernet-manager <address_of_SupernetManager_contract>
    ```

10. Initial staking on rootchain - each validator needs to do initial staking on rootchain (StakeManager) contract. **This command is for testing purposes only.**

    ```bash
    $ polygon-edge polybft stake --data-dir ./test-chain-1 --supernet-id <supernet_id_from_genesis> \
    --amount <amount_of_tokens_to_stake> \
    --stake-manager <address_of_StakeManager_contract> --stake-token <address_of_erc20_token_used_for_staking>
    ```

11. Do mint and premine for relayer node. **These commands should only be executed if non-mintable erc20 token is used**

    ```bash
    $ polygon-edge bridge mint-erc20 \ 
    --erc20-token <address_of_native_root_erc20_token> \
    --private-key <hex_encoded_private_key_of_token_deployer> \
    --addresses <address_of_relayer_node> \
    --amounts <ammount_of_tokens_to_mint_to_relayer>
    ```

     ```bash
    $ polygon-edge rootchain premine \ 
    --erc20-token <address_of_native_root_erc20_token> \
       --root-erc20-predicate <address_of_root_erc20_predicate_on_root> \
       --supernet-manager <address_of_CustomSupernetManager_contract_on_root> \
       --private-key <hex_encoded_private_key_of_relayer_node> \
       --amount <ammount_of_tokens_to_premine>
    ```

12. Finalize genesis validator set on rootchain (SupernetManager) contract. This is done after all validators from genesis do initial staking on rootchain, and it's a final step that is required before starting the child chain. This needs to be done by the deployer of SupernetManager contract (the user that run the deploy command). He can use either its hex encoded private key, or data-dir flag if he has secerets initialized. If enable-staking flag is provided, validators will be able to continue staking on rootchain. If not, genesis validators will not be able update its stake or unstake, nor will newly registered validators after genesis will be able to stake tokens on the rootchain. Enabling of staking can be done through this command, or later after the child chain starts.

    ```bash
    $ polygon-edge polybft supernet --private-key <hex_encoded_rootchain_account_private_key_of_supernetManager_deployer> \
    --genesis <path_to_genesis_file> \
    --supernet-manager <address_of_SupernetManager_contract> \
    --finalize-genesis --enable-staking
    ```

13. Run (child chain) cluster, consisting of 4 Edge clients in this particular example

    ```bash
    $ polygon-edge server --data-dir ./test-chain-1 --chain genesis.json --grpc-address :5001 --libp2p :30301 --jsonrpc :9545 \
    --seal --log-level DEBUG

    $ polygon-edge server --data-dir ./test-chain-2 --chain genesis.json --grpc-address :5002 --libp2p :30302 --jsonrpc :10002 \
    --seal --log-level DEBUG

    $ polygon-edge server --data-dir ./test-chain-3 --chain genesis.json --grpc-address :5003 --libp2p :30303 --jsonrpc :10003 \
    --seal --log-level DEBUG
    
    $ polygon-edge server --data-dir ./test-chain-4 --chain genesis.json --grpc-address :5004 --libp2p :30304 --jsonrpc :10004 \
    --seal --log-level DEBUG
    ```

    It is possible to run child chain nodes in "relayer" mode. It allows automatic execution of deposit events on behalf of users.
    In order to start node in relayer mode, it is necessary to supply `--relayer` flag:

    ```bash
    $ polygon-edge server --data-dir ./test-chain-1 --chain genesis.json --grpc-address :5001 --libp2p :30301 --jsonrpc :9545 \
    --seal --log-level DEBUG --relayer
    ```
