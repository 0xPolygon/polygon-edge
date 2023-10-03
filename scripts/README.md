# Run servers from local binary

## Prerequisites

When deploying with `polybft` consensus, there are some additional dependencies:

* [go 1.20.x](https://go.dev/dl/)
* [jq](https://jqlang.github.io/jq)
* [curl](https://everything.curl.dev/get)

## Local development

Running `polygon-edge` from local binary can be done very easily by using provided `scripts` folder.

* `scripts/cluster ibft` - deploy environment with `ibft` consensus
* `scripts/cluster polybft` - deploy environment with `polybft` consensus

## Customization

Use `scripts/cluster` script to customize chain parameters.
It already has some default parameters, which can be easily modified.
These are the `genesis` parameters from the official [docs](https://wiki.polygon.technology/docs/edge/operate/param-reference/).

Primarily, the `--premine` parameter needs to be edited (`createGenesis` function) to include the accounts that the user has access to.

## Considerations

### Live console

The servers are run in foreground, meaning that the terminal console that is running the script must remain active.
To stop the servers - `Ctrl/Cmd + C`.
To interact with the chain use another terminal or run a dockerized environment by following the instructions in `docker/README.md`.

### Production

This is **NOT** a production ready deployment. It is to be used in *development* / *test* environments only.
For production usage, please check out the official [docs](https://wiki.polygon.technology/docs/edge/).
