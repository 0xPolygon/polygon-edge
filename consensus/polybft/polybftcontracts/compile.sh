#!/bin/bash

set -x

if [[ "$(docker images -q v3_contracts_builder:latest 2> /dev/null)" == "" ]]; then
		docker build -t v3_contracts_builder -f $PWD/consensus/polybft/polybftcontracts/Dockerfile $PWD/consensus/polybft/polybftcontracts/
fi

docker run \
  --name v3_build_contracts \
  -v $PWD/consensus/polybft/polybftcontracts/contracts:/usr/src/app/contracts \
  -v $PWD/consensus/polybft/polybftcontracts/artifacts:/usr/src/app/artifacts \
  -v $PWD/consensus/polybft/polybftcontracts/cache:/usr/src/app/cache \
  v3_contracts_builder
docker rm -f v3_build_contracts
