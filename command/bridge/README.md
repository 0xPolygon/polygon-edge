# Bridge helper command

This is a helper command, which allows sending deposits from root to child chain and make withdrawals from child chain to root chain.

## Deposit ERC20

This is a helper command which deposits ERC20 tokens from the root chain to the child chain

```bash
$ polygon-edge bridge deposit-erc20 \
    --sender-key <hex_encoded_depositor_private_key> \
    --receivers <receivers_addresses> \
    --amounts <amounts> \
    --root-token <root_erc20_token_address> \
    --root-predicate <root_erc20_predicate_address> \
    --json-rpc <json_rpc_endpoint>
    [--minter-key <hex_encoded_minter_account_private_key>]
```

**Note:** in case `minter-key` is provided, tokens are going to be minted to sender account. Note that provided minter private key must belong to the account which has minter role.

## Withdraw ERC20

This is a helper command which withdraws ERC20 tokens from the child chain to the root chain

```bash
$ polygon-edge bridge withdraw-erc20 \
    --sender-key <hex_encoded_txn_sender_private_key> \
    --receivers <receivers_addresses> \
    --amounts <amounts> \
    --child-predicate <child_erc20_predicate_address> \
    [--child-token <child_erc20_token_address>] \
    --json-rpc <json_rpc_endpoint>
```

## Exit

This is a helper command which qeuries child chain for exit event proof and sends an exit transaction to ExitHelper smart contract.

```bash
$ polygon-edge bridge exit \
    --sender-key <hex_encoded_txn_sender_private_key> \
    --exit-helper <exit_helper_address> \
    --exit-id <exit_event_id> \
    --root-json-rpc <root_chain_json_rpc_endpoint> \
    --child-json-rpc <child_chain_json_rpc_endpoint>
```

**Note:** for using test account provided by Geth dev instance, use `--test` flag. In that case `--sender-key` flag can be omitted and test account is used as an exit transaction sender.
