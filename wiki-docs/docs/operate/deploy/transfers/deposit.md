
## Prerequisites

You'll need to have a successful bridge deployment to make any cross-chain transactions. If you haven't done so already, check out the local deployment guide [<ins>here</ins>](../local-chain.md).

:::caution Key management and secure values
When passing values to run transactions, it is important to keep sensitive values like private keys and API keys secure.

<b>The sample commands provided in this guide use sample private keys for demonstration purposes only, in order to show the format and expected value of the parameter. It is important to note that hardcoding or directly passing private keys should never be done in a development or production environment.</b>

<details>
<summary>Here are some options for securely storing and retrieving private keys ↓</summary>

- **<ins>Environment Variables</ins>:** You can store the private key as an environment variable and access it in your code. For example, in Linux, you can set an environment variable like this: `export PRIVATE_KEY="my_private_key"`. Then, in your code, you can retrieve the value of the environment variable using `os.Getenv("PRIVATE_KEY")`.

- **<ins>Configuration Files</ins>:** You can store the private key in a configuration file and read it in your session. Be sure to keep the configuration file in a secure location and restrict access to it.

- **<ins>Vaults and Key Management Systems</ins>:** If you are working with sensitive data, you might consider using a vault or key management system like a keystore to store your private keys. These systems provide additional layers of security and can help ensure that your private keys are kept safe.

</details>

Regardless of how a private key is stored and retrieved, it's important to keep it secure and not expose it unnecessarily.

:::

### Depositing from the Rootchain

<div align="center">
  <img src="/img/edge/bridge-deposit-rootchain.excalidraw.png" alt="bridge" width="100%" height="30%" />
</div>

### Depositing from the Childchain

<div align="center">
  <img src="/img/edge/bridge-deposit-childchain.excalidraw.png" alt="bridge" width="100%" height="30%" />
</div>

For a detailed understanding of how bridging works, please refer to the sequence diagrams available [<ins>here</ins>](https://github.com/0xPolygon/polygon-edge/blob/develop/docs/bridge/sequences.md).

<!-- ===================================================================================================================== -->
<!-- ===================================================================================================================== -->
<!-- ===================================================== GUIDE TABS ==================================================== -->
<!-- ===================================================================================================================== -->
<!-- ===================================================================================================================== -->

<Tabs
defaultValue="20"
values={[
{ label: 'ERC-20', value: '20', },
{ label: 'ERC-721', value: '721', },
{ label: 'ERC-1155', value: '1155', },
]
}>

<!-- ===================================================================================================================== -->
<!-- ==================================================== ERC-20  ======================================================== -->
<!-- ===================================================================================================================== -->

<TabItem value="20">

This command deposits ERC-20 tokens from a rootchain to a Edge-powered chain. 

- Replace `hex_encoded_depositor_private_key` with the private key of the account that will be depositing the tokens.
- Replace `receivers_addresses` with a comma-separated list of Ethereum addresses that will receive the tokens.
- Replace `amounts` with a comma-separated list of token amounts to be deposited for each receiver.
- Replace `root_erc20_token_address` with the address of the ERC-20 token contract on the rootchain.
- Replace `root_erc20_predicate_address` with the address of the ERC-20 predicate contract on the rootchain.
- Replace `root_chain_json_rpc_endpoint` with the JSON-RPC endpoint of the rootchain.
- Replace `minter-key` with the private key of the minter account. If the private key is provided, tokens are minted to the sender account prior to depositing them.

```bash
./polygon-edge bridge deposit-erc20 \
    --sender-key <hex_encoded_depositor_private_key> \
    --receivers <receivers_addresses> \
    --amounts <amounts> \
    --root-token <root_erc20_token_address> \
    --root-predicate <root_erc20_predicate_address> \
    --json-rpc <root_chain_json_rpc_endpoint>
    --minter-key <minter account hex encoded private key>
```

<details>
<summary>Example ↓</summary>

In this example, we're depositing ERC-20 tokens to a test Edge instance:

- We're using a private key of `0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef`.
- We're depositing tokens to two receiver addresses: `0x1111111111111111111111111111111111111111` and `0x2222222222222222222222222222222222222222`.
- We're depositing `100` tokens to the first receiver and `200` tokens to the second receiver.
- The address of the ERC-20 token contract on the rootchain is `0x123456789abcdef0123456789abcdef01234567`.
- The address of the ERC-20 predicate contract on the rootchain is `0x23456789abcdef0123456789abcdef012345678`.
- The JSON-RPC endpoint for the rootchain is `http://root-chain-json-rpc-endpoint.com:8545`.
- We're using a minter key of `0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef`.

```bash
./polygon-edge bridge deposit-erc20 \
    --sender-key 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
    --receivers 0x1111111111111111111111111111111111111111,0x2222222222222222222222222222222222222222 \
    --amounts 100,200 \
    --root-token 0x123456789abcdef0123456789abcdef01234567 \
    --root-predicate 0x23456789abcdef0123456789abcdef012345678 \
    --json-rpc http://root-chain-json-rpc-endpoint.com:8545
    --minter-key 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

</details>
</TabItem>

<!-- ===================================================================================================================== -->
<!-- =================================================== ERC-721  ======================================================== -->
<!-- ===================================================================================================================== -->

<TabItem value="721">

This command deposits ERC-721 tokens from a rootchain to a Edge-powered chain.

- Replace `hex_encoded_depositor_private_key` with the private key of the account that will be depositing the tokens.
- Replace `receivers_addresses` with a comma-separated list of Ethereum addresses that will receive the tokens.
- Replace `token_ids` with a comma-separated list of token IDs that will be sent to the receivers' accounts.
- Replace `root_erc721_token_address` with the address of the ERC-721 token contract on the rootchain.
- Replace `root_erc721_predicate_address` with the address of the ERC-721 predicate contract on the rootchain.
- Replace `root_chain_json_rpc_endpoint` with the JSON-RPC endpoint of the rootchain.
- Replace `minter-key` with the private key of the minter account. If the private key is provided, tokens are minted to the sender account prior to depositing them.

```bash
./polygon-edge bridge deposit-erc721 \
    --sender-key <hex_encoded_depositor_private_key> \
    --receivers <receivers_addresses> \
    --token-ids <token_ids> \
    --root-token <root_erc721_token_address> \
    --root-predicate <root_erc721_predicate_address> \
    --json-rpc <root_chain_json_rpc_endpoint>
    --minter-key <minter account hex encoded private key>
```

<details>
<summary>Example ↓</summary>

In this example, we're depositing ERC-721 tokens to a test Edge instance:

- We're using a private key of `0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef`.
- We're depositing tokens with IDs `123` and `456` to two receiver addresses: `0x1111111111111111111111111111111111111111` and `0x2222222222222222222222222222222222222222`.
- The address of the ERC-721 token contract on the rootchain is `0x0123456789abcdef0123456789abcdef01234567`.
- The address of the ERC-721 predicate contract on the rootchain is `0x0123456789abcdef0123456789abcdef01234568`.
- The JSON-RPC endpoint for the rootchain is `https://rpc-mainnet.maticvigil.com`.
- We're using a minter key of `0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef`.

```bash
./polygon-edge bridge deposit-erc721 \
    --sender-key 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
    --receivers 0x1111111111111111111111111111111111111111,0x2222222222222222222222222222222222222222 \
    --token-ids 123,456 \
    --root-token 0x0123456789abcdef0123456789abcdef01234567 \
    --root-predicate 0x0123456789abcdef0123456789abcdef01234568 \
    --json-rpc https://rpc-mainnet.maticvigil.com
    --minter-key 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```
</details>
</TabItem>

<!-- ===================================================================================================================== -->
<!-- ==================================================== ERC-1155 ======================================================= -->
<!-- ===================================================================================================================== -->

<TabItem value="1155">

This command deposits ERC-1155 tokens from the rootchain to the Edge-powered chain.

- Replace `depositor_private_key` with the private key of the account that will be depositing the tokens.
- Replace `receivers_addresses` with a comma-separated list of Ethereum addresses that will receive the tokens.
- Replace `amounts` with a comma-separated list of token amounts to be deposited for each receiver.
- Replace `token_ids` with a comma-separated list of token IDs to be deposited.
- Replace `root_erc1155_token_address` with the address of the ERC-1155 token contract on the rootchain.
- Replace `root_erc1155_predicate_address` with the address of the ERC-1155 predicate contract on the rootchain.
- Replace `root_chain_json_rpc_endpoint` with the JSON-RPC endpoint of the rootchain.
- Replace `minter-key` with the private key of the minter account. If the private key is provided, tokens are minted to the sender account prior to depositing them.

```bash
./polygon-edge bridge deposit-erc1155 \
    --sender-key <depositor_private_key> \
    --receivers <receivers_addresses> \
    --amounts <amounts> \
    --token-ids <token_ids> \
    --root-token <root_erc1155_token_address> \
    --root-predicate <root_erc1155_predicate_address> \
    --json-rpc <root_chain_json_rpc_endpoint>
    --minter-key <minter account hex encoded private key>
```

<details>
<summary>Example ↓</summary>

In this example, we're depositing ERC-1155 tokens to a test Edge instance:

- We're using a private key of `0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef`.
- We're depositing tokens to two receiver addresses: `0x0123456789abcdef0123456789abcdef01234567` and `0x89abcdef0123456789abcdef0123456789abcdef`.
- We're depositing `10` tokens to the first receiver and `20` tokens to the second receiver.
- The address of the ERC-1155 token contract on the rootchain is `0x0123456789abcdef0123456789abcdef01234567`.
- The address of the ERC-1155 predicate contract on the rootchain is `0x89abcdef0123456789abcdef0123456789abcdef`.
- The JSON-RPC endpoint for the rootchain is `http://localhost:8545`.
- We're using a minter key of `0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef`.

```bash
./polygon-edge bridge deposit-erc1155 \
    --sender-key 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
    --receivers 0x0123456789abcdef0123456789abcdef01234567,0x89abcdef0123456789abcdef0123456789abcdef \
    --amounts 10,20 \
    --token-ids 1,2 \
    --root-token 0x0123456789abcdef0123456789abcdef01234567 \
    --root-predicate 0x89abcdef0123456789abcdef0123456789abcdef \
    --json-rpc http://localhost:8545
    --minter-key 0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

</details>
</TabItem>
</Tabs>
