In this section, we'll configure the associated rootchain of the Edge-powered chain and deploy the necessary rootchain core contracts.

After generating the initial chain state for your Edge-powered chain, the next step is to connect and initialize the rootchain contracts. This can be done using either a demo Geth instance or any EVM-compatible rootchain. The demo Geth instance is a local instance of a Geth node running in development mode, which simulates the Ethereum network and is **only intended for testing purposes**.

## 1. Deploy and Initialize Rootchain Contracts

<!-- ===================================================================================================================== -->
<!-- ==================================================== ROOTCHAIN TABS ================================================= -->
<!-- ===================================================================================================================== -->

<Tabs
defaultValue="geth"
values={[
{ label: 'Demo Geth Instance', value: 'geth', },
{ label: 'EVM-Compatible Rootchain', value: 'rootchain', },
]
}>

<!-- ==================================================== GETH ROOTCHAIN ================================================= -->

<TabItem value="geth">

### i. Start the Geth Node

The `polygon-edge` rootchain server command starts an ethereum/client-go container, which runs a new Geth node.
To do this, open a new terminal session and run:

  ```bash
  ./polygon-edge rootchain server
  ```

<details>
<summary>Output example ↓</summary>

You should see output similar to the following, indicating that the rootchain server is now running:

  ```bash
  {"status":"Pulling from 0xpolygon/go-ethereum-console","id":"latest"}
  {"status":"Digest: sha256:6aad124b6775b96d05c94850dcafde45911f7bb2b473328dc4a792b1ffb2bdb6"}
  {"status":"Status: Image is up to date for ghcr.io/0xpolygon/go-ethereum-console:latest"}
  INFO [05-04|09:48:51.496] Starting Geth in ephemeral dev mode...
  WARN [05-04|09:48:51.498] You are running Geth in --dev mode. Please note the following:

    1. This mode is only intended for fast, iterative development without assumptions on
       security or persistence.
    2. The database is created in memory unless specified otherwise. Therefore, shutting down
       your computer or losing power will wipe your entire block data and chain state for
       your dev environment.
    3. A random, pre-allocated developer account will be available and unlocked as
       eth.coinbase, which can be used for testing. The random dev account is temporary,
       stored on a ramdisk, and will be lost if your machine is restarted.
    4. Mining is enabled by default. However, the client will only seal blocks if transactions
       are pending in the mempool. The miner's minimum accepted gas price is 1.
    5. Networking is disabled; there is no listen-address, the maximum number of peers is set
       to 0, and discovery is disabled.

  INFO [05-04|09:48:51.515] Maximum peer count                       ETH=50 LES=0 total=50
  INFO [05-04|09:48:51.524] Smartcard socket not found, disabling    err="stat /run/pcscd/pcscd.comm: no such file or directory"
  INFO [05-04|09:48:51.576] Set global gas cap                       cap=50,000,000
  INFO [05-04|09:48:52.676] Using developer account                  address=0xd8aC7c1C8A8F34392aD45C250489DeE5D1dC5F51
  INFO [05-04|09:48:52.682] Allocated cache and file handles         database=/eth1data/geth/chaindata cache=512.00MiB handles=524,288
  INFO [05-04|09:48:52.807] Opened ancient database                  database=/eth1data/geth/chaindata/ancient/chain readonly=false
  INFO [05-04|09:48:52.824] Allocated trie memory caches             clean=154.00MiB dirty=256.00MiB
  INFO [05-04|09:48:52.827] Allocated cache and file handles         database=/eth1data/geth/chaindata               cache=512.00MiB handles=524,288
  INFO [05-04|09:48:52.987] Opened ancient database                  database=/eth1data/geth/chaindata/ancient/chain readonly=false
  INFO [05-04|09:48:53.003] Initialising Ethereum protocol           network=1337 dbversion=<nil>
  INFO [05-04|09:48:53.005] Writing custom genesis block
  INFO [05-04|09:48:53.018] Persisted trie from memory database      nodes=12 size=1.82KiB time=2.892875ms gcnodes=0 gcsize=0.00B gctime=0s livenodes=1 livesize=0.00B
  INFO [05-04|09:48:53.029]
  INFO [05-04|09:48:53.029] ---------------------------------------------------------------------------------------------------------------------------------------------------------
  INFO [05-04|09:48:53.030] Chain ID:  1337 (unknown)
  INFO [05-04|09:48:53.030] Consensus: Clique (proof-of-authority)
  INFO [05-04|09:48:53.030]
  INFO [05-04|09:48:53.030] Pre-Merge hard forks:
  INFO [05-04|09:48:53.031]  - Homestead:                   0        (https://github.com/ethereum/execution-specs/blob/master/network-upgrades/mainnet-upgrades/homestead.md)
  INFO [05-04|09:48:53.031]  - Tangerine Whistle (EIP 150): 0        (https://github.com/ethereum/execution-specs/blob/master/network-upgrades/mainnet-upgrades/tangerine-whistle.md)
  INFO [05-04|09:48:53.031]  - Spurious Dragon/1 (EIP 155): 0        (https://github.com/ethereum/execution-specs/blob/master/network-upgrades/mainnet-upgrades/spurious-dragon.md)
  INFO [05-04|09:48:53.031]  - Spurious Dragon/2 (EIP 158): 0        (https://github.com/ethereum/execution-specs/blob/master/network-upgrades/mainnet-upgrades/spurious-dragon.md)
  INFO [05-04|09:48:53.032]  - Byzantium:                   0        (https://github.com/ethereum/execution-specs/blob/master/network-upgrades/mainnet-upgrades/byzantium.md)
  INFO [05-04|09:48:53.032]  - Constantinople:              0        (https://github.com/ethereum/execution-specs/blob/master/network-upgrades/mainnet-upgrades/constantinople.md)
  INFO [05-04|09:48:53.032]  - Petersburg:                  0        (https://github.com/ethereum/execution-specs/blob/master/network-upgrades/mainnet-upgrades/petersburg.md)
  INFO [05-04|09:48:53.032]  - Istanbul:                    0        (https://github.com/ethereum/execution-specs/blob/master/network-upgrades/mainnet-upgrades/istanbul.md)
  INFO [05-04|09:48:53.032]  - Muir Glacier:                0        (https://github.com/ethereum/execution-specs/blob/master/network-upgrades/mainnet-upgrades/muir-glacier.md)
  INFO [05-04|09:48:53.033]  - Berlin:                      0        (https://github.com/ethereum/execution-specs/blob/master/network-upgrades/mainnet-upgrades/berlin.md)
  INFO [05-04|09:48:53.033]  - London:                      0        (https://github.com/ethereum/execution-specs/blob/master/network-upgrades/mainnet-upgrades/london.md)
  INFO [05-04|09:48:53.033]
  INFO [05-04|09:48:53.033] The Merge is not yet available for this network!
  INFO [05-04|09:48:53.033]  - Hard-fork specification: https://github.com/ethereum/execution-specs/blob/master/network-upgrades/mainnet-upgrades/paris.md
  INFO [05-04|09:48:53.034] ---------------------------------------------------------------------------------------------------------------------------------------------------------
  INFO [05-04|09:48:53.034]
  INFO [05-04|09:48:53.039] Loaded most recent local header          number=0 hash=368c9f..43bc46 td=1 age=54y1mo1w
  INFO [05-04|09:48:53.040] Loaded most recent local full block      number=0 hash=368c9f..43bc46 td=1 age=54y1mo1w
  INFO [05-04|09:48:53.040] Loaded most recent local fast block      number=0 hash=368c9f..43bc46 td=1 age=54y1mo1w
  WARN [05-04|09:48:53.043] Failed to load snapshot                  err="missing or corrupted snapshot"
  INFO [05-04|09:48:53.045] Rebuilding state snapshot
  INFO [05-04|09:48:53.048] Resuming state snapshot generation       root=1e8d18..71da09 accounts=0 slots=0 storage=0.00B dangling=0 elapsed=2.311ms
  INFO [05-04|09:48:53.060] Regenerated local transaction journal    transactions=0 accounts=0
  INFO [05-04|09:48:53.064] Generated state snapshot                 accounts=10 slots=0 storage=412.00B dangling=0 elapsed=18.140ms
  INFO [05-04|09:48:53.066] Gasprice oracle is ignoring threshold set threshold=2
  WARN [05-04|09:48:53.071] Error reading unclean shutdown markers   error="leveldb: not found"
  WARN [05-04|09:48:53.072] Engine API enabled                       protocol=eth
  WARN [05-04|09:48:53.072] Engine API started but chain not configured for merge yet
  INFO [05-04|09:48:53.073] Stored checkpoint snapshot to disk       number=0 hash=368c9f..43bc46
  INFO [05-04|09:48:53.076] Starting peer-to-peer node               instance=Geth/v1.11.0-unstable-b590faff-20221007/linux-amd64/go1.18.6
  WARN [05-04|09:48:53.077] P2P server will be useless, neither dialing nor listening
  INFO [05-04|09:48:53.137] New local node record                    seq=1,683,193,733,117 id=bb78e0f3d24eccb1 ip=127.0.0.1 udp=0 tcp=0
  INFO [05-04|09:48:53.145] Started P2P networking                   self=enode://d1a6c5a13a7469707feec8af30254f4fba1035fee47c56514790020dacbcbaeace23943e0838e7dd6555f6f723cf1d4c9ff3a00ebdf2bb796c71787fbd6407cc@127.0.0.1:0
  INFO [05-04|09:48:53.151] IPC endpoint opened                      url=/eth1data/geth.ipc
  INFO [05-04|09:48:53.159] Generated JWT secret                     path=/eth1data/geth/jwtsecret
  INFO [05-04|09:48:53.163] HTTP server started                      endpoint=[::]:8545 auth=false prefix= cors= vhosts=localhost
  INFO [05-04|09:48:53.163] WebSocket enabled                        url=ws://[::]:8546
  INFO [05-04|09:48:53.168] WebSocket enabled                        url=ws://127.0.0.1:8551
  INFO [05-04|09:48:53.168] HTTP server started                      endpoint=127.0.0.1:8551 auth=true  prefix= cors=localhost vhosts=localhost
  INFO [05-04|09:48:53.174] Transaction pool price threshold updated price=0
  INFO [05-04|09:48:53.175] Updated mining threads                   threads=0
  INFO [05-04|09:48:53.175] Transaction pool price threshold updated price=1
  INFO [05-04|09:48:53.175] Etherbase automatically configured       address=0xd8aC7c1C8A8F34392aD45C250489DeE5D1dC5F51
  INFO [05-04|09:48:53.179] Commit new sealing work                  number=1 sealhash=c40800..8372ed uncles=0 txs=0 gas=0 fees=0 elapsed=2.295ms
  INFO [05-04|09:48:53.180] Commit new sealing work                  number=1 sealhash=c40800..8372ed uncles=0 txs=0 gas=0 fees=0 elapsed=3.644ms
  INFO [05-04|09:48:53.185] Successfully sealed new block            number=1 sealhash=c40800..8372ed hash=75c2e6..45c554 elapsed=6.768ms
  ```

</details>

This will start the rootchain server on the default JSON-RPC port of `8545`.

### ii. Deploy StakeManager Contract

:::caution

If you have already deployed the StakeManager, you may skip the step and move onto [<ins>Deploy rootchain contracts</ins>](#deploy-rootchain-contracts).

:::

If the `StakeManager` hasn't been deployed to the rootchain, you need to carry out this step. This command also contains a test flag. This flag is strictly for testing purposes, and its usage results in deploying a mock ERC-20 token that will serve for staking.

  ```bash
  ./polygon-edge polybft stake-manager-deploy \
    --proxy-contracts-admin 0xaddressOfProxyContractsAdmin \
    --private-key <hex_encoded_rootchain_account_private_key> \
    --genesis ./genesis.json \
    --jsonrpc http://127.0.0.1:8545 \
    --stake-token 0xaddressOfStakeToken \
    --test
  ```

<details>
<summary>Flags ↓</summary>

| Flag                         | Description                                                                   | Example                                         |
|------------------------------|-------------------------------------------------------------------------------|-------------------------------------------------|
| `--config`                   | The path to the SecretsManager config file. If omitted, the local FS secrets manager is used. | `--config /path/to/config`                  |
| `--data-dir`                 | The directory for the Polygon Edge data if the local FS is used.              | `--data-dir test-chain-`                     |
| `--genesis`                  | Genesis file path, which contains chain configuration.                        | `--genesis ./genesis.json`                    |
| `-h, --help`                 | Help for stake-manager-deploy.                                                |                                                 |
| `--jsonrpc`                  | The JSON-RPC interface.                                                       | `--jsonrpc http://0.0.0.0:8545`              |
| `--private-key`              | Hex-encoded private key of the account which executes rootchain commands.     | `--private-key <PRIVATE_KEY>`                |
| `--proxy-contracts-admin`    | Admin for proxy contracts.                                                    | `--proxy-contracts-admin <PROXY_CONTRACTS_ADMIN>` |
| `--stake-token`              | Address of ERC20 token used for staking on rootchain.                         | `--stake-token <STAKE_TOKEN_ADDRESS>`         |
| `--test`                     | Indicates if the command is run in test mode. If test mode is used, the contract will be deployed using a test account, and a test stake ERC20 token will be deployed for staking. | `--test` |

</details>

## 2. Deployment Considerations

To deploy the rootchain contracts, we use the `polygon-edge rootchain deploy` command. Before deployment, consider the following actions:

### i. Funding Required for Nodes

Before initializing the contracts on the rootchain, we need to make sure that the nodes are funded with sufficient funds to cover the gas cost of deploying the contracts. Otherwise, the initialization process may fail due to a lack of funds. 

Note that the demo server already funds the default test account. If you're not omitting `--deployer-key`, ensure that you pass the correct key; otherwise, you may encounter an error due to insufficient funds, such as the following:

```bash
failed to deploy rootchain contracts: {"code":-32000,"message":"INTERNAL_ERROR: insufficient funds"}
```

You can also create a rootchain wallet and fund the nodes by using `polygon-cli`.
Follow the steps outlined [<ins>here</ins>](https://github.com/maticnetwork/polygon-cli).

### ii. Using an Existing ERC-20 as the Native Gas Token

If you already have an ERC-20 token deployed on the rootchain that you want to use as the gas token, you can specify its address using the `--erc20-token` flag when deploying the rootchain contracts. For example:

<details>
<summary>Example ↓</summary>

```bash
./polygon-edge rootchain deploy \
  --genesis ./genesis.json \
  --json-rpc http://127.0.0.1:8545 \
  --erc20-token <ERC20_TOKEN_ADDRESS>
```

</details>

Replace `<ERC20_TOKEN_ADDRESS>` with the address of your existing ERC-20 token on the rootchain.

To deposit a desired amount and mint it on the childchain, please refer to the guidelines outlined in the deposit guide [<ins>here</ins>](transfers/deposit.md).

### iii. Proxy Contract Admin

The introduction of the proxy contract admin in v1.3 brings about new considerations for deployment and contract interaction.

### Issue with Proxy Contract Fallback

A notable issue arises when the proxy contract's admin attempts to fallback to the proxy target. Specifically, the initialization on the StakeManager seems to be invoked using the admin account, which is set by the `--proxy-contracts-admin` flag. 

> According to the design of the [<ins>TransparentUpgradeableProxy</ins>](https://docs.openzeppelin.com/contracts/4.x/api/proxy#TransparentUpgradeableProxy), the admin account for the proxy contract is prohibited from calling any function on the implementation contract. This design choice ensures that the admin account's privileges are strictly limited to administrative tasks, preventing potential misuse or unintended interactions.

### Recommendations

1. **Exclusive Use of Admin Account**: Ensure that the address specified in the `--proxy-contracts-admin` flag is used exclusively for administrative functions. This includes tasks such as updating the implementation contract address and modifying the admin.

2. **Avoid Using Admin as Deployer**: Ensure that the address used in the `--proxy-contracts-admin` flag is not employed as a contract deployer. This is especially important in cases like the stake manager deployment.

3. **Restricted Function Calls**: The admin account should never be used to invoke any function on the implementation contract. This restriction is in line with the design of the `TransparentUpgradeableProxy` and ensures that the admin account remains solely for administrative tasks.

## 3. Deploy Rootchain Contracts

Using the `--deployer-key` flag, you will need to replace `<hex_encoded_deployer_private_key>` with the hex-encoded private key of the deployer account that will be used to deploy the smart contracts. If you omit the `--deployer-key` option, the default account in your local client will be used.

> The user is responsible for providing the deployer key, which should correspond to an address with sufficient funds for deployment. It is recommended to ensure the account is pre-funded before initiating the deployment process.

To run the deployment in test mode and use the test account provided by the Geth dev instance as the depositor, add the `--test` flag. In this case, you may omit the `--deployer-key` flag, and the default test account will be used as the depositor.

<details>
<summary>Flags ↓</summary>

| Flag                         | Description                                                                   | Example                                         |
|------------------------------|-------------------------------------------------------------------------------|-------------------------------------------------|
| `--deployer-key`             | Hex-encoded private key of the account which deploys rootchain contracts     | `--deployer-key <PRIVATE_KEY>`                  |
| `--erc20-token`              | Existing root chain root native token address                                | `--erc20-token <ERC_20_ADDRESS>`                |
| `--genesis`                  | Genesis file path, which contains chain configuration (default "./genesis.json") | `--genesis ./genesis.json`                    |
| `-h, --help`                 | Help for deploy                                                               |                                                 |
| `--json-rpc`                 | The JSON RPC rootchain IP address (default "http://127.0.0.1:8545")           | `--json-rpc http://127.0.0.1:8545`            |
| `--proxy-contracts-admin`    | Admin for proxy contracts                                                     | `--proxy-contracts-admin <PROXY_CONTRACTS_ADMIN>` |
| `--stake-manager`            | Address of stake manager contract                                             | `--stake-manager <STAKE_MANAGER_ADDRESS>`     |
| `--stake-token`              | Address of ERC20 token used for staking on rootchain                         | `--stake-token <STAKE_TOKEN_ADDRESS>`         |
| `--test`                     | Indicates whether rootchain contracts deployer is hardcoded test account     | `--test`                                        |

</details>

  ```bash
  ./polygon-edge rootchain deploy \
    --deployer-key <hex_encoded_rootchain_account_private_key> \
    --stake-manager <address_of_stake_manager_contract> \
    --stake-token 0xaddressOfStakeToken \
    --proxy-contracts-admin 0xaddressOfProxyContractsAdmin \
    --genesis ./genesis.json \
    --json-rpc http://127.0.0.1:8545 \
    --test
  ```

The above example will get the `stake-manager` and `stake-token` addresses directly from the `genesis.json` file. 

<details>
<summary>Core contract deployment output example</summary>

```bash
[ROOTCHAIN - CONTRACTS DEPLOYMENT] started... Rootchain JSON RPC address http://127.0.0.1:8545.


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = RootERC20
Contract (address) = 0x6FE03c2768C9d800AF3Dedf1878b5687FE120a27
Transaction (hash) = 0x5c77b2fbc658c97ceba964efd512f6fb01224ca0f81b6f97fbc0f258c7ae3b2b


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = RootERC721
Contract (address) = 0x3d46A809D5767B81a8836f0E79145ba615A2Dd61
Transaction (hash) = 0xdb3051d76de9296e3e2f8e723b85bc679e183bdbd47820506af61828bec17da3


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = RootERC1155
Contract (address) = 0x72E1C51FE6dABF2e3d5701170cf5aD3620E6B8ba
Transaction (hash) = 0x99f8897fe1d921b58aac44fef3d35e986186dee8d2ea8f06b55ca3c5e775c431


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = StateSender
Contract (address) = 0x436604426F31A05f905C64edc973E575BdB46471
Transaction (hash) = 0xcb1d9674f1c928527a470d74532a6fe1e755e64909c8ecd5cce014d7330fb500


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = CheckpointManager
Contract (address) = 0x947a581B2713F58A8145201DA41BCb6aAE90196B
Transaction (hash) = 0xf6bb5054971c0d0a135ae63767a39d4cd6f1401a961e6abcfca47351576a1235


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = BLS
Contract (address) = 0x1BfAdFDc7554f618665e3EAE7C22DE2B5ab54786
Transaction (hash) = 0xecad816ee9c8cee2bf646dd077ca26ac9d07f1593967c3b94ff7fa1110e0cd73


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = BN256G2
Contract (address) = 0x22C246401ed6e52C525644659C5304aed63516C7
Transaction (hash) = 0xf220bb6dcd40eef63df0517cb4ed768dfcb5f432644937b7dcaedcc7b4b5342e


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = ExitHelper
Contract (address) = 0x88d3678C1e99Fc0b699fCA4cf2BC1c2C75C7f272
Transaction (hash) = 0xa77415d8d06f6fd0973cd844ba67582afd34808c9943a9c2f8305a4f53ea66de


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = RootERC20Predicate
Contract (address) = 0xB3A64e1ffB0867E93665Da1052b3dbAb427A538C
Transaction (hash) = 0xdada762e15613a41b3cd37bc8f1c6750d2498f9bd743769549e8b4b6fbba07e1


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = ERC20Template
Contract (address) = 0xaCB3Eb2f3c167B56410F0351B6C6EBac9256f553
Transaction (hash) = 0xb53e9bc41aca03ea51cd82de944bd84618c25976b475bba9615eef2bf449c5d3


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = RootERC721Predicate
Contract (address) = 0xA1DFe8536732EB98BBCA36A7f97C72e3395EaB8E
Transaction (hash) = 0x4ee43d7754f03e4322ac8eb29bfd2c8410e837f42a786ae0f13f6aee4795c815


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = ERC721Template
Contract (address) = 0x8d83F76FB303d30d35E1A8FAafB69294C8bD4069
Transaction (hash) = 0x242d30e721e5a9a4324f92e5874a62de6bc65089b51013a4fbac6033cdf2a6fc


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = RootERC1155Predicate
Contract (address) = 0x0e3C79887960455083c5F063035C723c61906811
Transaction (hash) = 0x89013a2cfe0c68b73c226cb917822159ae68167b1f8a178e1e63dd8077e245d0


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = ERC1155Template
Contract (address) = 0x7e5BB8F3721C594Af6aB04D5bDf5C52742F37403
Transaction (hash) = 0x3415e2fb208fb0307746d3a0a35794a5de759fd77109d6a043dea7420c8da230


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = StakeManager
Contract (address) = 0x811068e4106f7A70D443684FF4927eC3940439Ec
Transaction (hash) = 0xe7ab6d4e5002a84b168b195affbf9cdf7bc1f67d05bbeb78e62e4517393626d7


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = CustomSupernetManager
Contract (address) = 0x75aA024A2292A3FD3C17d67b54B3d00435437246
Transaction (hash) = 0xbe80f099313c6505176a6bc888062ccb124deb83f2e413b0bf076ae5d6b9f6e5

[ROOTCHAIN - CONTRACTS DEPLOYMENT] StakeManager contract is initialized

[ROOTCHAIN - CONTRACTS DEPLOYMENT] CustomSupernetManager contract is initialized

[ROOTCHAIN - CONTRACTS DEPLOYMENT] [VALIDATORS]
Address=0x61324166B0202DB1E7502924326262274Fa4358F; Balance=1000000; P2P Multi addr=/ip4/127.0.0.1/tcp/30301/p2p/16Uiu2HAmMYyzK7c649Tnn6XdqFLP7fpPB2QWdck1Ee9vj5a7Nhg8; BLS Key=06d8d9e6af67c28e85ac400b72c2e635e83234f8a380865e050a206554049a222c4792120d84977a6ca669df56ff3a1cf1cfeccddb650e7aacff4ed6c1d4e37b055858209f80117b3c0a6e7a28e456d4caf2270f430f9df2ba37221f23e9bbd313c9ef488e1849cc5c40d18284d019dde5ed86770309b9c24b70ceff6167a6ca;
Address=0xFE5E166BA5EA50c04fCa00b07b59966E6C2E9570; Balance=1000000000000000000000000; P2P Multi addr=/ip4/127.0.0.1/tcp/30302/p2p/16Uiu2HAmLXVapjR2Yx3B1taCmHnckQ1ph2xrawBjW2kvSErps9CX; BLS Key=0601da8856a6d3d3bb0f3bcbb90ea7b8c0db8271b9203e6123c6804aa3fc5f810be33287968ca1af2be11839516850a6ffef2337d99e679b7531efbbea2e3bf727a053c0cbede71da3d5f489b6ad862ccd8bb0bfb7fa379e3395d3b1142594a73020e87d63c298a3a4eba0ace65727f8659bab6389b9448b72512db72bbe937f;
[ROOTCHAIN - CONTRACTS DEPLOYMENT] Validators hash: 0x9d31cd8a803b09a9c5e054301977b1f5b758c56f811351e937a0a3792e2ef8b1
[ROOTCHAIN - CONTRACTS DEPLOYMENT] CheckpointManager contract is initialized

[ROOTCHAIN - CONTRACTS DEPLOYMENT] ExitHelper contract is initialized

[ROOTCHAIN - CONTRACTS DEPLOYMENT] RootERC20Predicate contract is initialized

[ROOTCHAIN - CONTRACTS DEPLOYMENT] RootERC721Predicate contract is initialized

[ROOTCHAIN - CONTRACTS DEPLOYMENT] RootERC1155Predicate contract is initialized

[ROOTCHAIN - CONTRACTS DEPLOYMENT] finished. All contracts are successfully deployed and initialized.
```

</details>

</TabItem>

<!-- =================================================== ANY ROOTCHAIN ================================================ -->

<TabItem value="rootchain">

### i. Deploy StakeManager Contract

:::caution

If you have already deployed the StakeManager, you may skip the step and move onto [<ins>Deploy rootchain contracts</ins>](#deploy-rootchain-contracts).

:::

This command includes a test flag, which is intended solely for testing scenarios. When this flag is used, a mock ERC-20 token is deployed for staking. However, in non-testing environments, remember to specify the `stake-token` flag with the address of the token that's already deployed on the rootchain and will be used for staking.

  ```bash
  ./polygon-edge polybft stake-manager-deploy \
    --proxy-contracts-admin 0xaddressOfProxyContractsAdmin \
    --deployer-key <hex_encoded_rootchain_account_private_key> \
    --genesis ./genesis.json \
    --jsonrpc http://127.0.0.1:8545 \
    --stake-token 0xaddressOfStakeToken \
    --test
  ```

<details>
<summary>Flags ↓</summary>

| Flag            | Description                                                                                            | Example |
|-----------------|--------------------------------------------------------------------------------------------------------|---------|
| `--config`      | Path to the SecretsManager config file, if omitted, the local FS secrets manager is used               |         |
| `--data-dir`    | Directory for the Polygon Edge data if the local FS is used                                            |         |
| `--genesis`     | Genesis file path, which contains chain configuration (default "./genesis.json")                       |         |
| `--jsonrpc`     | The JSON-RPC interface (default "0.0.0.0:8545")                                                        |         |
| `--private-key` | Hex-encoded private key of the account which executes rootchain commands                               |         |
| `--stake-token` | Address of ERC20 token used for staking on rootchain                                                   |         |
| `--test`        | Contract will be deployed using test account and a test stake ERC20 token will be deployed for staking |         |

</details>

## 2. Deployment Considerations

To deploy the rootchain contracts, we use the `polygon-edge rootchain deploy` command. Before deployment, consider the following actions:

### i. Funding Required for Nodes

Before initializing the contracts on the rootchain, we need to make sure that the nodes are funded with sufficient funds to cover the gas cost of deploying the contracts. Otherwise, the initialization process may fail due to a lack of funds. 

Note that the demo server already funds the default test account. If you're not omitting `--deployer-key`, ensure that you pass the correct key; otherwise, you may encounter an error due to insufficient funds, such as the following:

```bash
failed to deploy rootchain contracts: {"code":-32000,"message":"INTERNAL_ERROR: insufficient funds"}
```

You can also create a rootchain wallet and fund the nodes by using `polygon-cli`.
Follow the steps outlined [<ins>here</ins>](https://github.com/maticnetwork/polygon-cli).

### ii. Using an Existing ERC-20 as the Native Gas Token

If you already have an ERC-20 token deployed on the rootchain that you want to use as the gas token, you can specify its address using the `--erc20-token` flag when deploying the rootchain contracts. For example:

<details>
<summary>Example ↓</summary>

```bash
./polygon-edge rootchain deploy \
  --genesis ./genesis.json \
  --json-rpc http://127.0.0.1:8545 \
  --erc20-token <ERC20_TOKEN_ADDRESS>
```

</details>

Replace `<ERC20_TOKEN_ADDRESS>` with the address of your existing ERC-20 token on the rootchain.

To deposit a desired amount and mint it on the childchain, please refer to the guidelines outlined in the deposit guide [<ins>here</ins>](transfers/deposit.md).

## 3. Deploy Rootchain Contracts

Using the `--deployer-key` flag, you will need to replace `<hex_encoded_deployer_private_key>` with the hex-encoded private key of the deployer account that will be used to deploy the smart contracts. If you omit the `--deployer-key` option, the default account in your local client will be used.

You also need to specify the path to the genesis file using the `--genesis` option, and the endpoint for the JSON-RPC endpoint for the rootchain using the `--json-rpc` option.

<details>
<summary>Flags ↓</summary>

| Flag                  | Description                                                              | Example                                       |
|-----------------------|--------------------------------------------------------------------------|-----------------------------------------------|
| `--deployer-key`      | Hex encoded private key of the account which deploys rootchain contracts | `--deployer-key <PRIVATE_KEY>`                |
| `--json-rpc`          | The JSON RPC rootchain IP address (e.g. http://127.0.0.1:8545)           | `--json-rpc http://127.0.0.1:8545`            |
| `--genesis`           | Genesis file path that contains chain configuration                      | `--genesis ./genesis.json`                    |
| `--erc20-token`       | Existing rootchain ERC-20 token address                                  | `--erc20-token <ERC_20_ADDRESS>`              |
| `--stake-manager`     | Address of stake manager contract                                        | `--stake-manager <STAKE_MANAGER_ADDRESS>`     |
| `--stake-token`       | Address of ERC20 token used for staking on rootchain                     | `--stake-token <STAKE_TOKEN_ADDRESS>`         |
| `--test`              | Indicates whether rootchain contracts deployer is hardcoded test account | `--test`                                      |

</details>

```bash
./polygon-edge rootchain deploy \
  --deployer-key 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
  --genesis ./genesis.json \
  --json-rpc http://127.0.0.1:8545 \
```

<details>
<summary>Core contract deployment output example ↓</summary>

```bash
[ROOTCHAIN - CONTRACTS DEPLOYMENT] started... Rootchain JSON RPC address http://127.0.0.1:8545.


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = RootERC20
Contract (address) = 0x6FE03c2768C9d800AF3Dedf1878b5687FE120a27
Transaction (hash) = 0x5c77b2fbc658c97ceba964efd512f6fb01224ca0f81b6f97fbc0f258c7ae3b2b


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = RootERC721
Contract (address) = 0x3d46A809D5767B81a8836f0E79145ba615A2Dd61
Transaction (hash) = 0xdb3051d76de9296e3e2f8e723b85bc679e183bdbd47820506af61828bec17da3


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = RootERC1155
Contract (address) = 0x72E1C51FE6dABF2e3d5701170cf5aD3620E6B8ba
Transaction (hash) = 0x99f8897fe1d921b58aac44fef3d35e986186dee8d2ea8f06b55ca3c5e775c431


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = StateSender
Contract (address) = 0x436604426F31A05f905C64edc973E575BdB46471
Transaction (hash) = 0xcb1d9674f1c928527a470d74532a6fe1e755e64909c8ecd5cce014d7330fb500


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = CheckpointManager
Contract (address) = 0x947a581B2713F58A8145201DA41BCb6aAE90196B
Transaction (hash) = 0xf6bb5054971c0d0a135ae63767a39d4cd6f1401a961e6abcfca47351576a1235


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = BLS
Contract (address) = 0x1BfAdFDc7554f618665e3EAE7C22DE2B5ab54786
Transaction (hash) = 0xecad816ee9c8cee2bf646dd077ca26ac9d07f1593967c3b94ff7fa1110e0cd73


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = BN256G2
Contract (address) = 0x22C246401ed6e52C525644659C5304aed63516C7
Transaction (hash) = 0xf220bb6dcd40eef63df0517cb4ed768dfcb5f432644937b7dcaedcc7b4b5342e


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = ExitHelper
Contract (address) = 0x88d3678C1e99Fc0b699fCA4cf2BC1c2C75C7f272
Transaction (hash) = 0xa77415d8d06f6fd0973cd844ba67582afd34808c9943a9c2f8305a4f53ea66de


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = RootERC20Predicate
Contract (address) = 0xB3A64e1ffB0867E93665Da1052b3dbAb427A538C
Transaction (hash) = 0xdada762e15613a41b3cd37bc8f1c6750d2498f9bd743769549e8b4b6fbba07e1


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = ERC20Template
Contract (address) = 0xaCB3Eb2f3c167B56410F0351B6C6EBac9256f553
Transaction (hash) = 0xb53e9bc41aca03ea51cd82de944bd84618c25976b475bba9615eef2bf449c5d3


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = RootERC721Predicate
Contract (address) = 0xA1DFe8536732EB98BBCA36A7f97C72e3395EaB8E
Transaction (hash) = 0x4ee43d7754f03e4322ac8eb29bfd2c8410e837f42a786ae0f13f6aee4795c815


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = ERC721Template
Contract (address) = 0x8d83F76FB303d30d35E1A8FAafB69294C8bD4069
Transaction (hash) = 0x242d30e721e5a9a4324f92e5874a62de6bc65089b51013a4fbac6033cdf2a6fc


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = RootERC1155Predicate
Contract (address) = 0x0e3C79887960455083c5F063035C723c61906811
Transaction (hash) = 0x89013a2cfe0c68b73c226cb917822159ae68167b1f8a178e1e63dd8077e245d0


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = ERC1155Template
Contract (address) = 0x7e5BB8F3721C594Af6aB04D5bDf5C52742F37403
Transaction (hash) = 0x3415e2fb208fb0307746d3a0a35794a5de759fd77109d6a043dea7420c8da230


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = StakeManager
Contract (address) = 0x811068e4106f7A70D443684FF4927eC3940439Ec
Transaction (hash) = 0xe7ab6d4e5002a84b168b195affbf9cdf7bc1f67d05bbeb78e62e4517393626d7


[ROOTCHAIN - DEPLOY CONTRACT]
Name               = CustomSupernetManager
Contract (address) = 0x75aA024A2292A3FD3C17d67b54B3d00435437246
Transaction (hash) = 0xbe80f099313c6505176a6bc888062ccb124deb83f2e413b0bf076ae5d6b9f6e5

[ROOTCHAIN - CONTRACTS DEPLOYMENT] StakeManager contract is initialized

[ROOTCHAIN - CONTRACTS DEPLOYMENT] CustomSupernetManager contract is initialized

[ROOTCHAIN - CONTRACTS DEPLOYMENT] [VALIDATORS]
Address=0x61324166B0202DB1E7502924326262274Fa4358F; Balance=1000000; P2P Multi addr=/ip4/127.0.0.1/tcp/30301/p2p/16Uiu2HAmMYyzK7c649Tnn6XdqFLP7fpPB2QWdck1Ee9vj5a7Nhg8; BLS Key=06d8d9e6af67c28e85ac400b72c2e635e83234f8a380865e050a206554049a222c4792120d84977a6ca669df56ff3a1cf1cfeccddb650e7aacff4ed6c1d4e37b055858209f80117b3c0a6e7a28e456d4caf2270f430f9df2ba37221f23e9bbd313c9ef488e1849cc5c40d18284d019dde5ed86770309b9c24b70ceff6167a6ca;
Address=0xFE5E166BA5EA50c04fCa00b07b59966E6C2E9570; Balance=1000000000000000000000000; P2P Multi addr=/ip4/127.0.0.1/tcp/30302/p2p/16Uiu2HAmLXVapjR2Yx3B1taCmHnckQ1ph2xrawBjW2kvSErps9CX; BLS Key=0601da8856a6d3d3bb0f3bcbb90ea7b8c0db8271b9203e6123c6804aa3fc5f810be33287968ca1af2be11839516850a6ffef2337d99e679b7531efbbea2e3bf727a053c0cbede71da3d5f489b6ad862ccd8bb0bfb7fa379e3395d3b1142594a73020e87d63c298a3a4eba0ace65727f8659bab6389b9448b72512db72bbe937f;
[ROOTCHAIN - CONTRACTS DEPLOYMENT] Validators hash: 0x9d31cd8a803b09a9c5e054301977b1f5b758c56f811351e937a0a3792e2ef8b1
[ROOTCHAIN - CONTRACTS DEPLOYMENT] CheckpointManager contract is initialized

[ROOTCHAIN - CONTRACTS DEPLOYMENT] ExitHelper contract is initialized

[ROOTCHAIN - CONTRACTS DEPLOYMENT] RootERC20Predicate contract is initialized

[ROOTCHAIN - CONTRACTS DEPLOYMENT] RootERC721Predicate contract is initialized

[ROOTCHAIN - CONTRACTS DEPLOYMENT] RootERC1155Predicate contract is initialized

[ROOTCHAIN - CONTRACTS DEPLOYMENT] finished. All contracts are successfully deployed and initialized.
```

</details>

</TabItem>
</Tabs>

## 4. Funding Validators on the Rootchain

Before deploying validator nodes on the Edge-powered chain, we need to ensure that the validators have sufficient funds on the rootchain network. It's crucial to have enough funds in the validator account, as they need to cover the gas fees associated with their transactions on the rootchain.

To fund the validators' accounts on the rootchain, we use the `polygon-edge rootchain fund` command. When executed with the appropriate flags, it will:

1. Retrieve the validator account from the secrets manager.
2. Send a transaction to fund the validator account on the rootchain with a value of 10,000 $token (the network's native token).
3. Repeat steps 1 and 2 for each validator.

**It's important to note that this command is also for testing purposes only.**

In a production environment, you would need to ensure that the validators have sufficient funds on the rootchain network to cover the gas fees associated with their transactions.

Here's an example of how to fund a validator account (replace address value with your own):

  ```bash
  ./polygon-edge rootchain fund --addresses 0x77C1eedFf656477462ce16084fE5Dc7F8a2507B9 --amounts 1000000000000000000
  ```

<details>
<summary>Funding output example ↓</summary>

  ```bash
  [ROOTCHAIN FUND]
  Validator (address) = 0x77C1eedFf656477462ce16084fE5Dc7F8a2507B9
  Transaction (hash)  = 0x294952b29e3e8ba15fefe28934090dc0d249d71d48c6b19ebe534840703030a9
  Is minted           = false
  ```

</details>

## 5. Next Steps

Now that we have successfully configured the initial setup of your Edge-powered chain, including both the childchain and rootchain, we can proceed to the next crucial step: rootchain staking and allowlisting through the configuration of the initial validator set.

To accomplish this, navigate to the [<ins>Configure the Initial Validator Set</ins>](genesis-validators.md) deployment guide.
