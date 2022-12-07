#!/bin/bash

function delete {
  rm -rf $1/blockchain
  rm -rf $1/consensus/polybft
  rm -rf $1/trie
}

delete test-chain-1
delete test-chain-2
delete test-chain-3
delete test-chain-4
