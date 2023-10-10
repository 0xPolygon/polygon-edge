# Deploying local docker cluster

## Prerequisites

* [Docker Engine](https://docs.docker.com/engine/install/) - Minimum version 20.10.23
* [Docker compose v.1.x](https://docs.docker.com/compose/install/) - Minimum version 1.29

### `polybft` consensus

When deploying with `polybft` consensus, there are some additional dependencies:

* [go 1.20.x](https://go.dev/dl/)

## Local development

Running `polygon-edge` local cluster with docker can be done very easily by using provided `scripts` folder
or by running `docker-compose` manually.

### Using provided `scripts` folder

***All commands need to be run from the repo root / root folder.***

* `scripts/cluster ibft --docker` - deploy environment with `ibft` consensus
* `scripts/cluster polybft --docker` - deploy environment with `polybft` consensus
* `scripts/cluster {ibft or polybft} --docker stop` - stop containers
* `scripts/cluster {ibft or polybft}--docker destroy` - destroy environment (delete containers and volumes)

### Using `docker-compose`

***All commands need to be run from the repo root / root folder.***

#### use `ibft` PoA consensus

* `export EDGE_CONSENSUS=ibft` - set `ibft` consensus
* `docker-compose -f ./docker/local/docker-compose.yml up -d --build` - deploy environment

#### use `polybft` consensus

* `export EDGE_CONSENSUS=polybft` - set `polybft` consensus
* `docker-compose -f ./docker/local/docker-compose.yml up -d --build` - deploy environment

#### stop / destroy

* `docker-compose -f ./docker/local/docker-compose.yml stop` - stop containers
* `docker-compose -f ./docker/local/docker-compose.yml down -v` - destroy environment

## Customization

Use `docker/local/polygon-edge.sh` script to customize chain parameters.
All parameters can be defined at the very beginning of the script, in the `CHAIN_CUSTOM_OPTIONS` variable.
It already has some default parameters, which can be easily modified.
These are the `genesis` parameters from the official [docs](https://wiki.polygon.technology/docs/edge/operate/param-reference/).  

Primarily, the `--premine` parameter needs to be edited to include the accounts that the user has access to.

## Considerations

### Build times

When building containers for the first time (or after purging docker build cache),
it might take a while to complete, depending on the hardware that the build operation is running on.

### Production

This is **NOT** a production ready deployment. It is to be used in *development* / *test* environments only.
For production usage, please check out the official [docs](https://wiki.polygon.technology/docs/edge/).
