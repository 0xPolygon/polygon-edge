#!/bin/sh

set -e

POLYGON_EDGE_BIN=./polygon-edge
CHAIN_CUSTOM_OPTIONS=$(tr "\n" " " << EOL
--block-gas-limit 10000000
--epoch-size 10
--chain-id 51001
--name polygon-edge-docker
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
                  echo "Generating IBFT secrets..."
                  secrets=$("$POLYGON_EDGE_BIN" secrets init --insecure --num 4 --data-dir /data/data- --json)
                  echo "Secrets have been successfully generated"

                  rm -f /data/genesis.json

                  echo "Generating IBFT Genesis file..."
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
                --native-token-config "Polygon:MATIC:18:true:$(echo "$secrets" | jq -r '.[0] | .address')" \
                --bootnode "/dns4/node-1/tcp/1478/p2p/$(echo "$secrets" | jq -r '.[0] | .node_id')" \
                --bootnode "/dns4/node-2/tcp/1478/p2p/$(echo "$secrets" | jq -r '.[1] | .node_id')" \
                --bootnode "/dns4/node-3/tcp/1478/p2p/$(echo "$secrets" | jq -r '.[2] | .node_id')" \
                --bootnode "/dns4/node-4/tcp/1478/p2p/$(echo "$secrets" | jq -r '.[3] | .node_id')"

              echo "Deploying stake manager..."
              "$POLYGON_EDGE_BIN" polybft stake-manager-deploy \
                --jsonrpc http://rootchain:8545 \
                --genesis /data/genesis.json \
                --test

              stakeManagerAddr=$(cat /data/genesis.json | jq -r '.params.engine.polybft.bridge.stakeManagerAddr')
              stakeToken=$(cat /data/genesis.json | jq -r '.params.engine.polybft.bridge.stakeTokenAddr')

              "$POLYGON_EDGE_BIN" rootchain deploy \
                --stake-manager ${stakeManagerAddr} \
                --stake-token ${stakeToken} \
                --json-rpc http://rootchain:8545 \
                --genesis /data/genesis.json \
                --test

              customSupernetManagerAddr=$(cat /data/genesis.json | jq -r '.params.engine.polybft.bridge.customSupernetManagerAddr')
              supernetID=$(cat /data/genesis.json | jq -r '.params.engine.polybft.supernetID')
              addresses="$(echo "$secrets" | jq -r '.[0] | .address'),$(echo "$secrets" | jq -r '.[1] | .address'),$(echo "$secrets" | jq -r '.[2] | .address'),$(echo "$secrets" | jq -r '.[3] | .address')"

              "$POLYGON_EDGE_BIN" rootchain fund \
                --json-rpc http://rootchain:8545 \
                --stake-token ${stakeToken} \
                --mint \
                --addresses ${addresses} \
                --amounts 1000000000000000000000000,1000000000000000000000000,1000000000000000000000000,1000000000000000000000000

              "$POLYGON_EDGE_BIN" polybft whitelist-validators \
                --addresses ${addresses} \
                --supernet-manager ${customSupernetManagerAddr} \
                --private-key aa75e9a7d427efc732f8e4f1a5b7646adcc61fd5bae40f80d13c8419c9f43d6d \
                --jsonrpc http://rootchain:8545

              counter=1
              while [ $counter -le 4 ]; do
                echo "Registering validator: ${counter}"

                "$POLYGON_EDGE_BIN" polybft register-validator \
                  --supernet-manager ${customSupernetManagerAddr} \
                  --data-dir /data/data-${counter} \
                  --jsonrpc http://rootchain:8545

                "$POLYGON_EDGE_BIN" polybft stake \
                  --data-dir /data/data-${counter} \
                  --amount 1000000000000000000000000 \
                  --supernet-id ${supernetID} \
                  --stake-manager ${stakeManagerAddr} \
                  --stake-token ${stakeToken} \
                  --jsonrpc http://rootchain:8545

                counter=$((counter + 1))
              done

              "$POLYGON_EDGE_BIN" polybft supernet \
                --private-key aa75e9a7d427efc732f8e4f1a5b7646adcc61fd5bae40f80d13c8419c9f43d6d \
                --supernet-manager ${customSupernetManagerAddr} \
                --stake-manager ${stakeManagerAddr} \
                --finalize-genesis-set \
                --enable-staking \
                --genesis /data/genesis.json \
                --jsonrpc http://rootchain:8545
              ;;
      esac
      ;;
   *)
      echo "Executing polygon-edge..."
      exec "$POLYGON_EDGE_BIN" "$@"
      ;;
esac
