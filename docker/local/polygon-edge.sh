#!/bin/sh

set -e

POLYGON_EDGE_BIN=./polygon-edge

case "$1" in

   "init")
      case "$2" in 
         "ibft")
              echo "Generating secrets..."
              secrets=$("$POLYGON_EDGE_BIN" secrets init --num 4 --data-dir /data/data- --json)
              echo "Secrets have been successfully generated"

              echo "Generating IBFT Genesis file..."
              cd /data && /polygon-edge/polygon-edge genesis \
                --dir genesis.json \
                --consensus ibft \
                --ibft-validators-prefix-path data- \
                --bootnode /dns4/node-1/tcp/1478/p2p/$(echo $secrets | jq -r '.[0] | .node_id') \
                --bootnode /dns4/node-2/tcp/1478/p2p/$(echo $secrets | jq -r '.[1] | .node_id')
              ;;
          "polybft")
              echo "Generating PolyBFT secrets..."
              secrets=$("$POLYGON_EDGE_BIN" polybft-secrets init --num 4 --data-dir /data/data- --json)
              echo "Secrets have been successfully generated"

              echo "Generating manifest..."
              "$POLYGON_EDGE_BIN" manifest --path /data/manifest.json --validators-path /data --validators-prefix data-

              echo "Generating PolyBFT Genesis file..."
              "$POLYGON_EDGE_BIN" genesis \
                --dir /data/genesis.json \
                --consensus polybft \
                --manifest /data/manifest.json \
                --validator-set-size=4 \
                --block-gas-limit 10000000 \
                --epoch-size 10 \
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
