# Bridge helper command

This is a helper command, which allows sending deposits from root to child chain and make withdrawals from child chain to root chain.

## Deposit
This is a helper command which bridges assets from rootchain to the child chain (allows depositing)

```bash
$ polygon-edge bridge deposit --token <token_type> --receivers <receivers_addresses> --amounts <amounts>
```