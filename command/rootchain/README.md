# RootChain Helper

## Start rootchain server

This command starts `ethereum/client-go` container which is basically geth node, 
and deploys the rootchain bridge and the checkpoint manager contracts.

```bash
$ polygon-edge rootchain server
```

## Fund initialized accounts

This command funds the initialized accounts via `polygon-edge secrets init ...` command.

```bash
$ polygon-edge rootchain fund --data-dir data-dir- --num 2
```
Or
```bash
$ polygon-edge rootchain fund --data-dir data-dir-1
```

## Deposit assets

This is a helper command which bridges assets from rootchain to the child chain (aka deposit workflow)

```bash
$ polygon-edge rootchain deposit --token <token_type> --receivers <receivers_addresses> --amounts <amounts>
```