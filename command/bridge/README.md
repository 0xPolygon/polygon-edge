# Bridge helper command

This is a helper command, which allows sending deposits from root to child chain and make withdrawals from child chain to root chain.

## Deposit
This is a helper command which bridges assets from the root chain to the child chain (allows depositing)

```bash
$ polygon-edge bridge deposit 
    --sender-key <hex_encoded_depositor_private_key>
    --token <token_type>
    --receivers <receivers_addresses>
    --amounts <amounts>
    --manifest <manifest_file_path>
    --json-rpc <root_chain_json_rpc_endpoint>
```

## Withdraw
This is a helper command which bridges assets from the child chain to the root chain (allows withdrawal)

```bash
$ polygon-edge bridge withdraw 
    --sender-key <hex_encoded_withdraw_sender_private_key>
    --token <token_type>
    --receivers <receivers_addresses>
    --amounts <amounts>
    --manifest <manifest_file_path>
    --json-rpc <child_chain_json_rpc_endpoint>
```