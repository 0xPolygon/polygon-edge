### Prerequisites
* [Docker Desktop](https://www.docker.com/products/docker-desktop/) - Docker 17.12.0+
* [Docker compose 2+](https://github.com/docker/compose/releases/tag/v2.14.1)
* [Make](https://www.gnu.org/software/make/) - if using `make` commands

## Local development
Running `polygon-edge` local cluster with docker can be done very easily by using provided `Makefile`
or by running `docker-compose` manually.

### Using `make`
***All commands need to be run from the repo root / root folder.***

* `make run-local` - deploy environment with `ibft` consensus
* `make run-local-polybft` - deploy environment with `polybft` consensus
* `make stop-local` - stop containers
* `make destroy-local` - destroy environment (delete containers and volumes)

### Using `docker-compose`
***All commands need to be run from the repo root / root folder.***

#### use `ibft` consensus
* `export EDGE_CONSENSUS=ibft` - set `ibft` consensus
* `docker-compose -f ./docker/local/docker-compose.yml up -d --build` - deploy environment

#### use `polybft` consensus
* `export EDGE_CONSENSUS=polybft` - set `polybft` consensus
* `docker-compose -f ./docker/local/docker-compose.yml up -d --build` - deploy environment

#### stop / destroy 
* `docker-compose -f ./docker/local/docker-compose.yml stop` - stop containers
* `docker-compose -f ./docker/local/docker-compose.yml down -v` - destroy environment

## Considerations

### Submodules
Before deploying `polybft` environment, `core-contracts` submodule needs to be downloaded.  
To do that simply run `make download-submodules`.

### Build times
When building containers for the first time (or after purging docker build cache)
it might take a while to complete, depending on the hardware that the build operation is running on.