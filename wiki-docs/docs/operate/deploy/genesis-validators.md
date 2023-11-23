In this section, we'll look at how to configure the initial rootchain validator set through allowlisting and by staking.

## 1. Allowlist validators on the rootchain

The `CustomSupernetManager` contract on the rootchain is responsible for managing the PolyBFT network.

Before validators can be registered on the `CustomSupernetManager` contract on the rootchain, they need to be allowlisted by the deployer of the `CustomSupernetManager` contract. Validators can register themselves, or a registrator can register them on their behalf. Once registered, validators can stake and start validating transactions.

This can be done using the `polygon-edge polybft whitelist-validators` command. The deployer can specify the hex-encoded private key of the `CustomSupernetManager` contract deployer or use the `--data-dir` flag if they have initialized their secrets.

<details>
<summary>Flags ↓</summary>

| Flag              | Description                                                                                      | Example                                     |
| -----------------| ------------------------------------------------------------------------------------------------| ------------------------------------------- |
| `--private-key`     | Hex-encoded private key of the account that deploys the SupernetManager contract                | `--private-key <hex_encoded_rootchain_account_private_key_of_CustomSupernetManager_deployer>`             |
| `--addresses`       | Comma-separated list of hex-encoded addresses of validators to be whitelisted                   | `--addresses 0x8a98f47a9820e3f3a6C16f44194F1d7eCCe3A110,0x8a98f47a9820e3f3a6C16f44194F1d7eCCe3A110` |
| --supernet-manager| Address of the SupernetManager contract on the rootchain                                        | `--supernet-manager 0x3c6f8c6Fd90b2Bee1E78E2B2D1e7aB6cFf9Dc113` |
| `--data-dir`        | Directory for the Polygon Edge data if the local FS is used                                     | `--data-dir ./polygon-edge/data`             |
| `--jsonrpc`         | JSON-RPC interface                                                                              | `--jsonrpc 0.0.0.0:8545`                    |
| `--config`          | Path to the SecretsManager config file. If omitted, the local FS secrets manager is used        | `--config /path/to/config/file.yaml`        |

</details>

In the following example command, we are using a placeholder private key for the `CustomSupernetManager` contract deployer.

> If running the demo Geth instance, the test account private key has been hardcoded: `aa75e9a7d427efc732f8e4f1a5b7646adcc61fd5bae40f80d13c8419c9f43d6d`.

The `--addresses` flag is a comma-separated list of the first two validators generated earlier. The `--supernet-manager` flag specifies the actual `CustomSupernetManager` contract address that was deployed.

```bash
./polygon-edge polybft whitelist-validators \
  --private-key 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
  --addresses 0x61324166B0202DB1E7502924326262274Fa4358F,0xFE5E166BA5EA50c04fCa00b07b59966E6C2E9570 \
  --supernet-manager 0x75aA024A2292A3FD3C17d67b54B3d00435437246
```

## 2. Register validators on the rootchain

Each validator needs to register themselves on the `CustomSupernetManager` contract. This is done using the `polygon-edge polybft register-validator` command. **Note that this command is for testing purposes only.**

<details>
<summary>Flags ↓</summary>

| Flag                          | Description                                                                                                       | Example                                                |
| -----------------------------| ----------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| `--config`                      | Path to the SecretsManager config file. If omitted, the local FS secrets manager is used.                          | `--config /path/to/config/file.yaml`                   |
| `--data-dir`                    | The directory path where the new validator key is stored.                                                         | `--data-dir /path/to/validator1`                       |                                                      |
| `--jsonrpc`                     | The JSON-RPC interface. Default is `0.0.0.0:8545`.                                                                 | `--jsonrpc 0.0.0.0:8545`                              |
| `--supernet-manager`            | Address of the SupernetManager contract on the rootchain.                                                          | `--supernet-manager 0x75aA024A2292A3FD3C17d67b54B3d00435437246`      |

</details>

```bash
./polygon-edge polybft register-validator --data-dir ./test-chain-1 \
  --supernet-manager $(cat genesis.json | jq -r '.params.engine.polybft.bridge.customSupernetManagerAddr')
```

## 3. Initial staking on the rootchain

Each validator needs to perform initial staking on the rootchain `StakeManager` contract. This is done using the `polygon-edge polybft stake` command. **Note that this command is for testing purposes only.**

<details>
<summary>Flags ↓</summary>

|| Flag                          | Description                                                                      | Example                                  |
| -----------------------------| --------------------------------------------------------------------------------- | ---------------------------------------- |
| `--amount `                     | The amount to stake                                                            | `--amount 5000000000000000000`           |
| `--supernet-id`                 | The ID of the supernet provided by stake manager on supernet registration      | `--chain-id 100`                         |
| `--config `                     | The path to the SecretsManager config file                                     | `--config /path/to/config/file.yaml`     |
| `--data-dir`                    | The directory for the Polygon Edge data                                        | `--data-dir ./polygon-edge/data`         |
| `--jsonrpc`                     | The JSON-RPC interface                                                         | `--jsonrpc 0.0.0.0:8545`                |
| `--stake-token `                | The address of ERC20 Token used for staking on rootchain                       | `--native-root-token 0x<token_address>`  |
| `--stake-manager`               | The address of the stake manager contract                                      | `--stake-manager 0x<manager_address>`   |

</details>

In the following example command, we use the validator key and the rootchain `StakeManager` contract address that was generated earlier. We also set the staking amount to `1000000000000000000` which is equivalent to 1 token. The `--stake-token` flag is used to specify the address of the native token of the rootchain.

:::info Staking requirement: wrapping a non-ERC-20 token

Edge allow for the customization of the gas token and mandate the use of ERC-20 tokens for staking instead of the rootchain's native token.

When performing rootchain staking on the Polygon PoS Mainnet, [<ins>WMATIC</ins>](https://polygonscan.com/token/0x0d500b1d8e8ef31e21c99d1db9a6444d3adf1270?a=0x68b3465833fb72a70ecdf485e0e4c7bd8665fc45) (wrapped MATIC) is the required token. This is due to the ERC-20 standard requirement for staking, which MATIC doesn't meet.

WMATIC is deployed at `0x0d500b1d8e8ef31e21c99d1db9a6444d3adf1270`.

This principle also applies to the Mumbai Testnet, where wrapped test MATIC must be used instead of test MATIC.

If you currently hold MATIC tokens, you can convert them to WMATIC through various methods. One common method is to use a decentralized exchange (DEX) like Uniswap, where you can swap your MATIC tokens for WMATIC. **Always ensure you're using a reputable platform for this conversion and double-check that the contract address for WMATIC is correct.**

<details>
<summary>Obtaining native root token address</summary>

For example, if you are using the Mumbai test network, you can obtain the address of the MATIC testnet token by sending a POST request to the Mumbai network's JSON-RPC endpoint:

```bash
curl <mumbai-rpc-endpoint> \
  -X POST \
  -H "Content-Type: application/json" \
  --data '{"jsonrpc":"2.0","method":"eth_call","params":[{"to":"<token-contract-address>","data":"0x06fdde03"},"latest"],"id":1}'
```

</details>

:::

```bash
./polygon-edge polybft stake \
  --data-dir ./test-chain-1 \
  --supernet-id $(cat genesis.json | jq -r '.params.engine.polybft.supernetID') \
  --amount 1000000000000000000 \
  --stake-manager $(cat genesis.json | jq -r '.params.engine.polybft.bridge.stakeManagerAddr') \
  --stake-token $(cat genesis.json | jq -r '.params.engine.polybft.bridge.stakeTokenAddr') \
```

## 4. Finalize validator set on the rootchain

After all validators from the genesis block have performed initial staking on the rootchain, the final step required before starting the chain is to finalize the genesis validator set on the `SupernetManager` contract on the rootchain. This can be done using the `polygon-edge polybft supernet` command.

The deployer of the `SupernetManager` contract can specify their hex-encoded private key or use the `--data-dir` flag if they have initialized their secrets. If the `--enable-staking` flag is provided, validators will be able to continue staking on the rootchain. If not, genesis validators will not be able to update their stake or unstake, nor will newly registered validators after genesis be able to stake tokens on the rootchain. The enabling of staking can be done through this command or later after the Edge-powered chain starts.

In the following example command, we use a placeholder hex-encoded private key of the `SupernetManager` contract deployer. The addresses of the `SupernetManager` and `StakeManager` contracts are the addresses that were generated earlier. We also use the `--finalize-genesis-set` and `--enable-staking` flags to enable staking and finalize the genesis state.

```bash
./polygon-edge polybft supernet 
  --private-key 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
  --genesis /path/to/genesis/file \
  --supernet-manager $(cat genesis.json | jq -r '.params.engine.polybft.bridge.customSupernetManagerAddr') \
  --stake-manager $(cat genesis.json | jq -r '.params.engine.polybft.bridge.stakeManagerAddr') \
  --finalize-genesis-set --enable-staking
```

## 5. Next Steps

With all the necessary configurations in place for the Edge-powered chain, we are ready to proceed with starting the chain.

Navigate to the [<ins>Start Your Chain</ins>](start-chain.md) deployment guide, which will provide you with instructions on how to initiate and launch the chain.
