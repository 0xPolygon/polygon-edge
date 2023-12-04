In this section, we'll go over how to unstake a validator's staked tokens and withdraw them to an external address.

## Unstake

:::info Check validator information

First, check your validator information by running the `polygon-edge polybft validator-info` command.

<details>
<summary>Flags ↓</summary>

| Flag | Description | Example |
|------|-------------|---------|
| `--chain-id` | ID of Supernet | `137` |
| `--config` | The path to the SecretsManager config file, if omitted, the local FS secrets manager is used | `/path/to/config.yaml` |
| `--data-dir` | The directory for the Polygon Edge data if the local FS is used | `/path/to/data/dir` |
| `-h`, `--help` | Help for validator-info | |
| `--jsonrpc` | The JSON-RPC interface (default "0.0.0.0:8545") | `http://localhost:8545` |
| `--stake-manager` | Address of stake manager contract | `0x123...` |
| `--supernet-manager` | Address of manager contract | `0x456...` |

</details>

```bash
./polygon-edge polybft validator-info --data-dir ./test-chain-1
```

This will show you information about your validator account, including the staked amount.

:::

To unstake, use the `polygon-edge polybft unstake` command.

<details>
<summary>Flags ↓</summary>

| Flag                | Description                                                | Example                               |
|---------------------|------------------------------------------------------------|---------------------------------------|
| --amount            | Amount to unstake from validator                            | --amount 1000                         |
| --config            | Path to the SecretsManager config file                     | --config /path/to/config/file        |
| --data-dir          | Directory for the Polygon Edge data if the local FS is used | --data-dir /path/to/data/dir          |
| --jsonrpc           | JSON-RPC interface                                         | --jsonrpc 0.0.0.0:8545                |

</details>

```bash
./polygon-edge polybft unstake \
  --account-dir <DATA_DIR> \
  --jsonrpc <JSONRPC_ADDR> \
  --amount <AMOUNT>
```

## Withdraw

Once you have successfully unstaked, you will need to withdraw your unstaked tokens from the Edge-powered chain to the rootchain. To do so, use the `polygon-edge polybft withdraw-child` command.

<details>
<summary>Flags ↓</summary>

| Flag | Description | Example |
|------|-------------|---------|
| `--config` | The path to the SecretsManager config file, if omitted, the local FS secrets manager is used | `~/secrets.json` |
| `--data-dir` | The directory for the Polygon Edge data if the local FS is used | `~/polygon-edge/data` |
| `--jsonrpc` | The JSON-RPC interface (default "0.0.0.0:8545") | `127.0.0.1:8545` |

</details>

```bash
./polygon-edge polybft withdraw-child \
    --account-dir <DATA_DIR> \
    --jsonrpc <JSONRPC_ADDR>
```

This command will bridge the unstaked amount to the rootchain (`StakeManager`), where the given amount of tokens will become released on the given contract.

#### Wait for Checkpoint

Next, you will need to wait for the exit event (bridge event) to be included in a checkpoint.
You can confirm that the checkpoint has been successfully processed by checking the processed checkpoints on a blockchain explorer.

### Send an Exit Transaction

Once the exit event has been included in a checkpoint, you can send an exit transaction to execute the transaction on the rootchain. To do so, use the `polygon-edge bridge exit` command.

<details>
<summary>Flags ↓</summary>

| Flag                 | Description                                                         | Example                              |
|----------------------|---------------------------------------------------------------------|--------------------------------------|
| --child-json-rpc     | The JSON RPC Supernet endpoint.                                  | --child-json-rpc=http://127.0.0.1:9545 |
| --exit-helper        | Address of ExitHelper smart contract on rootchain.                 | --exit-helper=<EXIT_HELPER_ADDRESS>  |
| --exit-id            | Supernet exit event ID.                                          | --exit-id=<EXIT_ID>                  |
| --root-json-rpc      | The JSON RPC rootchain endpoint.                                   | --root-json-rpc=http://127.0.0.1:8545 |
| --sender-key         | Hex encoded private key of the account which sends exit transaction to the rootchain. | --sender-key=<SENDER_KEY> |
| --test               | Test indicates whether exit transaction sender is hardcoded test account. | --test                              |

</details>

  ```bash
  ./polygon-edge bridge exit \
      --sender-key <hex_encoded_txn_sender_private_key> \
      --exit-helper <exit_helper_address> \
      --exit-id <exit_event_id> \
      --root-json-rpc <root_chain_json_rpc_endpoint> \
      --child-json-rpc <child_chain_json_rpc_endpoint>
  ```

This will trigger the withdrawal of the specified amount of tokens from the rootchain.

### Withdraw from Root

Finally, you can withdraw your tokens from the rootchain to your address by using the `polygon-edge polybft withdraw-root` command.

<details>
<summary>Flags ↓</summary>

| Flag                   | Description                             | Example                            |
|------------------------|-----------------------------------------|------------------------------------|
| --amount               | amount to withdraw                      | --amount 1000000000000000000       |
| --config               | path to the SecretsManager config file  | --config /path/to/config           |
| --data-dir             | directory for the Polygon Edge data     | --data-dir /path/to/data/dir       |
| --jsonrpc              | JSON-RPC interface                       | --jsonrpc 0.0.0.0:8545             |
| --stake-manager        | address of stake manager contract        | --stake-manager 0x123abc           |
| --to                   | address where to withdraw                | --to 0x456def                      |

</details>

  ```bash
  ./polygon-edge polybft withdraw-root \
    --account-dir <DATA_DIR> \
    --to <RECIPIENT_ADDRESS> \
    --amount <AMOUNT> \
    --stake-manager <STAKE_MANAGER_ADDRESS> \
    --jsonrpc <BRIDGE_JSONRPC_ADDR>
  ```

This will transfer your withdrawn tokens back to the specified withdrawal address.
