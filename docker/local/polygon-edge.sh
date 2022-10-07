#!/bin/sh

set -e

POLYGON_EDGE_BIN=./polygon-edge

case "$1" in

   "init")
      echo "Generating secrets..."
      secrets=$("$POLYGON_EDGE_BIN" secrets init --num 4 --data-dir data- --json)
      echo "Secrets have been successfully generated"

      echo "Generating genesis file..."
      "$POLYGON_EDGE_BIN" genesis \
        --dir /genesis/genesis.json \
        --consensus ibft \
        --ibft-validators-prefix-path data- \
        --bootnode /dns4/node-1/tcp/1478/p2p/$(echo $secrets | jq -r '.[0] | .node_id') \
        --bootnode /dns4/node-2/tcp/1478/p2p/$(echo $secrets | jq -r '.[1] | .node_id')
      echo "Genesis file has been successfully generated"
      ;;

   *)
      echo "Executing polygon-edge..."
      exec "$POLYGON_EDGE_BIN" "$@"
      ;;

esac
