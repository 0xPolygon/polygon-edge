# Bridge helper command

This is a helper command, which allows sending deposits from root to child chain and make withdrawals from child chain to root chain.

## Deposit ERC20
This is a helper command which deposits ERC20 tokens from the root chain to the child chain

```bash
$ polygon-edge bridge deposit-erc20
    --sender-key <hex_encoded_depositor_private_key>
    --receivers <receivers_addresses>
    --amounts <amounts>
    --root-token <root_erc20_token_address>
    --root-predicate <root_erc20_predicate_address>
    --json-rpc <root_chain_json_rpc_endpoint>
```

## Withdraw ERC20
This is a helper command which withdraws ERC20 tokens from the child chain to the root chain

```bash
$ polygon-edge bridge withdraw-erc20
    --sender-key <hex_encoded_withdraw_sender_private_key>
    --receivers <receivers_addresses>
    --amounts <amounts>
    --child-predicate <rchild_erc20_predicate_address>
    [--child-token <child_erc20_token_address>]
    --json-rpc <child_chain_json_rpc_endpoint>
```

## Exit
This is a helper command which qeuries child chain for exit event proof and sends an exit transaction to ExitHelper smart contract.

```bash
$ polygon-edge bridge exit
    --sender-key <hex_encoded_withdraw_sender_private_key>
    --exit-helper <exit_helper_address>
    --event-id <exit_event_id>
    --epoch <epoch_in_which_exit_event_got_processed>
    --checkpoint-block <epoch_in_which_exit_event_got_processed>
    --root-json-rpc <root_chain_json_rpc_endpoint>
    --child-json-rpc <child_chain_json_rpc_endpoint>
```
