#!/usr/bin/env bash

rm -r -f e2e-logs*
rm -r -f test-cha*
sudo rm -r -f test-rootchain
rm genesis.json
go run . polybft-secrets --data-dir test-chain- --num 4
go run . rootchain server
