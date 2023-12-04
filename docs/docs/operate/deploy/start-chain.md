
To run an Edge cluster, we use the `polygon-edge server` command with the following options:

<details>
<summary>Flags ↓</summary>

| Flag                             | Description                                                                                                                                 | Example                                    |
|----------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------|--------------------------------------------|
| `--access-control-allow-origins` | The CORS header indicating whether any JSON-RPC response can be shared with the specified origin.                                           | `--access-control-allow-origins "*"`       |
| `--block-gas-target`             | The target block gas limit for the chain.                                                                                                   | `--block-gas-target "0x0"`                 |
| `--chain`                        | The genesis file used for starting the chain.                                                                                               | `--chain "./genesis.json"`                 |
| `--config`                       | The path to the CLI config.                                                                                                                 | `--config "/path/to/config.json"`          |
| `--data-dir`                     | The data directory used for storing Polygon Edge client data.                                                                               | `--data-dir "/path/to/data-dir"`           |
| `--dns`                          | The host DNS address which can be used by a remote peer for connection.                                                                     | `--dns "example.com"`                      |
| `--grpc-address`                 | The GRPC interface.                                                                                                                         | `--grpc-address "127.0.0.1:9632"`          |
| `--json-rpc-batch-request-limit` | Max length to be considered when handling JSON-RPC batch requests.                                                                          | `--json-rpc-batch-request-limit 20`        |
| `--json-rpc-block-range-limit`   | Max block range to be considered when executing JSON-RPC requests that consider fromBlock/toBlock values.                                   | `--json-rpc-block-range-limit 1000`        |
| `--jsonrpc`                      | The JSON-RPC interface.                                                                                                                     | `--jsonrpc "0.0.0.0:8545"`                 |
| `--libp2p`                       | The address and port for the libp2p service.                                                                                                | `--libp2p "127.0.0.1:1478"`                |
| `--log-level`                    | The log level for console output.                                                                                                           | `--log-level "INFO"`                       |
| `--log-to`                       | Write all logs to the file at specified location instead of writing them to console.                                                        |` --log-to "/path/to/log-file.log"`         |
| `--max-enqueued`                 | Maximum number of enqueued transactions per account.                                                                                        | `--max-enqueued 128`                       |
| `--max-inbound-peers`            | The client's max number of inbound peers allowed.                                                                                           | `--max-inbound-peers 32`                   |
| `--max-outbound-peers`           | The client's max number of outbound peers allowed.                                                                                          | `--max-outbound-peers 8`                   |
| `--max-peers`                    | The client's max number of peers allowed.                                                                                                   | `--max-peers 40`                           |
| `--max-slots`                    | Maximum slots in the pool.                                                                                                                  | `--max-slots 4096`                         |
| `--nat`                          | The external IP address without port, as can be seen by peers.                                                                              | `--nat "203.0.113.1"`                      |
| `--no-discover`                  | Prevent the client from discovering other peers.                                                                                            | `--no-discover`                            |
| `--num-block-confirmations`      | Minimal number of child blocks required for the parent block to be considered final.                                                        | `--num-block-confirmations 64`             |
| `--price-limit`                  | The minimum gas price limit to enforce for acceptance into the pool.                                                                        | `--price-limit 0`                          |
| `--prometheus`                   | The address and port for the Prometheus instrumentation service. If only port is defined, it will bind to all available network interfaces. |`--prometheus 0.0.0.0:9090`                 |
| `--relayer`                      | Start the state sync relayer service. PolyBFT only.                                                                                         |                                            |
| `--restore`                      | The path to the archive blockchain data to restore on initialization.                                                                       | `--restore /path/to/archive`               |
| `--seal`                         | The flag indicating that the client should seal blocks.                                                                                     |                                            |
| `--secrets-config`               | The path to the SecretsManager config file. Used for Hashicorp Vault. If omitted, the local FS secrets manager is used.                     | `--secrets-config /path/to/secrets/config` |

</details>

  ```bash
  ./polygon-edge server --data-dir ./test-chain-1 --chain genesis.json --grpc-address :5001 --libp2p :30301 --jsonrpc :10001 --seal --log-level DEBUG

  ./polygon-edge server --data-dir ./test-chain-2 --chain genesis.json --grpc-address :5002 --libp2p :30302 --jsonrpc :10002 --seal --log-level DEBUG

  ./polygon-edge server --data-dir ./test-chain-3 --chain genesis.json --grpc-address :5003 --libp2p :30303 --jsonrpc :10003 --seal --log-level DEBUG

  ./polygon-edge server --data-dir ./test-chain-4 --chain genesis.json --grpc-address :5004 --libp2p :30304 --jsonrpc :10004 --seal --log-level DEBUG
  ```

<details>
<summary>Dialing output example</summary>

  ```bash
  [ROOTCHAIN FUND]
  Validator (address) = 0x0D09C4A285fdde3D6e5aD5DE819E3478554646D3
  Transaction (hash)  = 0xb587d3fa31f8bc59ecc807145d95d76a454967e28223d0f3b82abdd6bd84c043

  [ROOTCHAIN FUND]
  Validator (address) = 0x30aC45469E94DE3645Eb4D8Ce102a3092ee76157
  Transaction (hash)  = 0x3e9b26da5e89aa8ca2b4935ce35ddedc1f8d9b37c56d5eb0f040787aa84a3bcb

  [ROOTCHAIN FUND]
  Validator (address) = 0x9E1bFa593cAcD77BfcF9a8Dda0462da251566ae0
  Transaction (hash)  = 0x1aa158ed2ba1e8ec98b1f4fd649c9a499b72c58a48b1a1dd9978ee16cc7fb741

  [ROOTCHAIN FUND]
  Validator (address) = 0x82e3D3e4222Cc872C5552363c86287B796312E27
  Transaction (hash)  = 0xd51e7f8b69071f88b5f7870c31c6942ed78c5c48f88594ed135f096b5f17a540
  ```

</details>
