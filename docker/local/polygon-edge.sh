#!/bin/sh

set -e

POLYGON_EDGE_BIN=./polygon-edge
GENESIS_PATH=/genesis/genesis.json

case "$1" in

   "init")
      if [ -f "$GENESIS_PATH" ]; then
          echo "Secrets have already been generated."
      else
          echo "Generating secrets..."
          secrets=$("$POLYGON_EDGE_BIN" secrets init --num 4 --data-dir data- --json)
          echo "Secrets have been successfully generated"

          echo "Generating genesis file..."
          "$POLYGON_EDGE_BIN" genesis \
            --dir "$GENESIS_PATH" \
            --consensus ibft \
            --ibft-validators-prefix-path data- \
            --bootnode /dns4/node-1/tcp/1478/p2p/$(echo $secrets | jq -r '.[0] | .node_id') \
            --bootnode /dns4/node-2/tcp/1478/p2p/$(echo $secrets | jq -r '.[1] | .node_id')
          echo "Genesis file has been successfully generated"
      fi
      ;;

   *)
      until [ -f "$GENESIS_PATH" ]
      do
          echo "Waiting 1s for genesis file $GENESIS_PATH to be created by init container..."
          sleep 1
      done
      echo "Executing polygon-edge..."
      exec "$POLYGON_EDGE_BIN" "$@"
      ;;

esac
