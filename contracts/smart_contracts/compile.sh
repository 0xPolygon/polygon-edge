#!/bin/bash

set -x

if [[ "$(docker images -q edge_contracts_builder:latest 2> /dev/null)" == "" ]]; then
		docker build -t edge_contracts_builder -f contracts/smart_contracts/Dockerfile contracts/smart_contracts
fi

docker run \
  --name edge_build_contracts \
  -v $PWD/contracts/smart_contracts/contracts:/usr/src/app/contracts \
  -v $PWD/contracts/smart_contracts/artifacts:/usr/src/app/artifacts \
  -v $PWD/contracts/smart_contracts/cache:/usr/src/app/cache \
  v3_contracts_builder
docker rm -f edge_build_contracts
