#!/bin/sh

set -e

POLYGON_EDGE_BIN=./polygon-edge

case "$1" in

   "init")
      echo "Generating secrets..."
      node1id=$("$POLYGON_EDGE_BIN" secrets init --data-dir data-1 | grep Node | awk -F ' ' '{print $4}')
      node2id=$("$POLYGON_EDGE_BIN" secrets init --data-dir data-2 | grep Node | awk -F ' ' '{print $4}')
      "$POLYGON_EDGE_BIN" secrets init --data-dir data-3
      "$POLYGON_EDGE_BIN" secrets init --data-dir data-4
      echo "Secrets have been successfully generated"

      echo "Generating genesis file..."
      "$POLYGON_EDGE_BIN" genesis \
        --dir /genesis/genesis.json \
        --consensus ibft \
        --ibft-validators-prefix-path data- \
        --bootnode /dns4/node-1/tcp/10001/p2p/"$node1id" \
        --bootnode /dns4/node-2/tcp/10001/p2p/"$node2id"
      echo "Genesis file has been successfully generated"
      ;;

   *)
      echo "Executing polygon-edge..."
      exec "$POLYGON_EDGE_BIN" "$@"
      ;;

esac
