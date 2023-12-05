In this section, we'll walkthrough how to stake WMATIC on the associated rootchain.

## i. Initial Staking on the Rootchain

Each validator needs to perform initial staking on the rootchain `StakeManager` contract. This is done using the `polygon-edge root stake` command. **Note that this command is for testing purposes only.**

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

In the following example command, we use the validator key and the rootchain `StakeManager` contract address that was generated earlier. We also set the staking amount to `1000000000000000000` which is equivalent to 1 token. The `--native-root-token` flag is used to specify the address of the native token of the rootchain.

:::info Staking requirement: wrapping a non-ERC-20 token

Edge allows for the customization of the gas token and mandate the use of ERC-20 tokens for staking instead of the rootchain's native token.

When performing rootchain staking on the Polygon PoS Mainnet, [<ins>WMATIC</ins>](https://polygonscan.com/token/0x0d500b1d8e8ef31e21c99d1db9a6444d3adf1270?a=0x68b3465833fb72a70ecdf485e0e4c7bd8665fc45) (wrapped MATIC) is the required token. This is due to the ERC-20 standard requirement for staking, which MATIC doesn't meet.

WMATIC is deployed at `0x0d500b1d8e8ef31e21c99d1db9a6444d3adf1270`.

This principle also applies to the Mumbai Testnet, where wrapped test MATIC must be used instead of test MATIC.

If you currently hold MATIC tokens, you can convert them to WMATIC through various methods. One common method is to use a decentralized exchange (DEX) like Uniswap, where you can swap your MATIC tokens for WMATIC. **Always ensure you're using a reputable platform for this conversion and double-check that the contract address for WMATIC is correct to ensure the security of your tokens.**

<details>
<summary>Obtaining native root token address</summary>

For example, if you are using the Mumbai test network, you can obtain the address of the MATIC testnet token by sending a GET request to the Mumbai network's JSON-RPC endpoint:

```bash
curl <mumbai-rpc-endpoint> \
-X POST \
-H "Content-Type: application/json" \
--data '{"jsonrpc":"2.0","method":"eth_contractAddress","params":["MaticToken"],"id":1}'
```

</details>

:::

```bash
./polygon-edge polybft stake \
--data-dir ./test-chain-1 \
--chain-id 100 \
--amount 1000000000000000000 \
--stake-manager 0x6ceCFe1Db48Ab97fA89b06Df6Bd0bBdE6E64e6F7 \
--native-root-token 0x559Dd13d8A3CAb3Ff127c408f2A08A8a2AEfC56c
```

## ii. Finalize Validator Set on the Rootchain

After all validators from the genesis block have performed initial staking on the rootchain, the final step required before starting the chain is to finalize the genesis validator set on the `SupernetManager` contract on the rootchain. This can be done using the `polygon-edge polybft supernet` command.

The deployer of the `SupernetManager` contract can specify their hex-encoded private key or use the `--data-dir` flag if they have initialized their secrets. If the `--enable-staking` flag is provided, validators will be able to continue staking on the rootchain. If not, genesis validators will not be able to update their stake or unstake, nor will newly registered validators after genesis be able to stake tokens on the rootchain. The enabling of staking can be done through this command or later after the chain starts.

In the following example command, we use a placeholder hex-encoded private key of the `SupernetManager` contract deployer. The addresses of the `SupernetManager` and `StakeManager` contracts are the addresses that were generated earlier. We also use the `--finalize-genesis` and `--enable-staking` flags to enable staking and finalize the genesis state.

```bash
   ./polygon-edge polybft supernet --private-key 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
   --genesis /path/to/genesis/file \
   --supernet-manager 0x75aA024A2292A3FD3C17d67b54B3d00435437246 \
   --stake-manager 0x811068e4106f7A70D443684FF4927eC3940439Ec \
   --finalize-genesis --enable-staking
```
