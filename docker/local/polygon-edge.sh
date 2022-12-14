#!/bin/sh

set -e

POLYGON_EDGE_BIN=./polygon-edge

case "$1" in

   "init")
      case "$2" in 
         "ibft")
              echo "Generating secrets..."
              secrets=$("$POLYGON_EDGE_BIN" secrets init --num 4 --data-dir data- --json)
              echo "Secrets have been successfully generated"
              echo "Generating IBFT Genesis file..."
              "$POLYGON_EDGE_BIN" genesis \
                --dir /genesis/genesis.json \
                --consensus ibft \
                --ibft-validators-prefix-path data- \
                --bootnode /dns4/node-1/tcp/1478/p2p/$(echo $secrets | jq -r '.[0] | .node_id') \
                --bootnode /dns4/node-2/tcp/1478/p2p/$(echo $secrets | jq -r '.[1] | .node_id')
              echo "Genesis file has been successfully generated"
              ;;
          "polybft")
              echo "Generating PolyBFT secrets..."
              secrets=$("$POLYGON_EDGE_BIN" polybft-secrets init --num 4 --data-dir data- --json)
              echo "Secrets have been successfully generated"
              ls .
              echo "generating PolyBFT Genesis file..."
              "$POLYGON_EDGE_BIN" genesis \
                --validator-prefix data- \
                --dir /genesis/genesis.json \
                --consensus polybft \
                --validator-set-size=4 \
                --bootnode /dns4/node-1/tcp/1478/p2p/$(echo $secrets | jq -r '.[0] | .node_id') \
                --bootnode /dns4/node-2/tcp/1478/p2p/$(echo $secrets | jq -r '.[1] | .node_id')
              ;;
      esac
      ;;

   *)
      echo "Executing polygon-edge..."
      exec "$POLYGON_EDGE_BIN" "$@"
      ;;

esac
