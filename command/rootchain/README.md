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

## Emit event

This command emits the event from the bridge side which invokes the wallets funding logic.

```bash
$ polygon-edge rootchain emit --contract <sidechain-bridge-contract> --wallets <wallets> --amounts <amounts>
```