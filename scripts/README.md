# Run servers form local binary

## Prerequisites

When deploying with `polybft` consensus, there are some additional dependencies:
* [npm](https://nodejs.org/en/)
* [go 1.18.x](https://go.dev/dl/)

## Local development
Running `polygon-edge` from local binary can be done very easily by using provided `scripts` folder.

* `scripts/cluster ibft` - deploy environment with `ibft` consensus
* `scripts/cluster polybft` - deploy environment with `polybft` consensus

## Customisation
Use `scripts/cluster` script to customize chain parameters.   
It already has some default parameters, which can be easily modified.
These are the `genesis` parameters from the official [docs](https://wiki.polygon.technology/docs/edge/get-started/cli-commands#genesis-flags).

Primarily, the `--premine` parameter needs to be edited (`createGenesis` function) to include the accounts that the user has access to.

## Considerations

### Live console
The servers are run in foreground, meaning that the terminal console that is running the script 
must remain active. To stop the servers - `Ctrl/Cmd + C`.    
To interact with the chain use another terminal or run a dockerized environment by following the instructions 
in `docker/README.md`.

### Submodules
Before deploying `polybft` environment, `core-contracts` submodule needs to be downloaded.  
To do that simply run `make download-submodules`.

### Production
This is **NOT** a production ready deployment. It is to be used in *development* / *test* environments only.       
For production usage, please check out the official [docs](https://wiki.polygon.technology/docs/edge/overview/). 