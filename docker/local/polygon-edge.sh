#!/bin/sh

set -e

POLYGON_EDGE_BIN=./polygon-edge
CHAIN_CUSTOM_OPTIONS=$(tr "\n" " " << EOL
--block-gas-limit 10000000
--epoch-size 10
--chain-id 51001
--name polygon-edge-docker
--burn-contract 0:0x0000000000000000000000000000000000000000
--premine 0x228466F2C715CbEC05dEAbfAc040ce3619d7CF0B:0xD3C21BCECCEDA1000000
--premine 0xca48694ebcB2548dF5030372BE4dAad694ef174e:0xD3C21BCECCEDA1000000
EOL
)

case "$1" in
   "init")
      case "$2" in 
          "ibft")
              if [ -f "$GENESIS_PATH" ]; then
                  echo "Secrets have already been generated."
              else
                  echo "Generating iBFT secrets..."
                  secrets=$("$POLYGON_EDGE_BIN" secrets init --insecure --num 4 --data-dir /data/data- --json)
                  echo "Secrets have been successfully generated"

                  rm -f /data/genesis.json

                  echo "Generating iBFT Genesis file..."
                  "$POLYGON_EDGE_BIN" genesis $CHAIN_CUSTOM_OPTIONS \
                    --dir /data/genesis.json \
                    --consensus ibft \
                    --ibft-validators-prefix-path data- \
                    --bootnode "/dns4/node-1/tcp/1478/p2p/$(echo "$secrets" | jq -r '.[0] | .node_id')" \
                    --bootnode "/dns4/node-2/tcp/1478/p2p/$(echo "$secrets" | jq -r '.[1] | .node_id')" \
                    --bootnode "/dns4/node-3/tcp/1478/p2p/$(echo "$secrets" | jq -r '.[2] | .node_id')" \
                    --bootnode "/dns4/node-4/tcp/1478/p2p/$(echo "$secrets" | jq -r '.[3] | .node_id')"
              fi
              ;;
          "polybft")
              echo "Generating PolyBFT secrets..."
              secrets=$("$POLYGON_EDGE_BIN" polybft-secrets init --insecure --num 4 --data-dir /data/data- --json)
              echo "Secrets have been successfully generated"

              rm -f /data/genesis.json

              echo "Generating PolyBFT genesis file..."
              "$POLYGON_EDGE_BIN" genesis $CHAIN_CUSTOM_OPTIONS \
                --dir /data/genesis.json \
                --consensus polybft \
                --validators-path /data \
                --validators-prefix data- \
                --reward-wallet 0xDEADBEEF:1000000 \
                --bootnode "/dns4/node-1/tcp/1478/p2p/$(echo "$secrets" | jq -r '.[0] | .node_id')" \
                --bootnode "/dns4/node-2/tcp/1478/p2p/$(echo "$secrets" | jq -r '.[1] | .node_id')" \
                --bootnode "/dns4/node-3/tcp/1478/p2p/$(echo "$secrets" | jq -r '.[2] | .node_id')" \
                --bootnode "/dns4/node-4/tcp/1478/p2p/$(echo "$secrets" | jq -r '.[3] | .node_id')"

              "$POLYGON_EDGE_BIN" rootchain server >/dev/null &
              ;;
      esac
      ;;
   *)
      echo "Executing polygon-edge..."
      exec "$POLYGON_EDGE_BIN" "$@"
      ;;

esac
