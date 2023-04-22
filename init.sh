#!/bin/bash

sudo rm -r test-rootchain
rm genesis.json
rm manifest.json
sudo rm -r test-chain-*/blockchain test-chain-*/trie test-chain-*/consensus/polybft test-chain-1/relayer.db

# go run . polybft-secrets --data-dir test-chain- --num 4 --insecure
go run . rootchain server 2>&1 | tee ./tmp/rootchain-server.log
