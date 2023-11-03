#!/bin/sh

set -e

POLYGON_EDGE_BIN=./polygon-edge
CHAIN_CUSTOM_OPTIONS=$(tr "\n" " " << EOL
--block-gas-limit 10000000
--epoch-size 10
--chain-id 51001
--name polygon-edge-docker
--premine 0x0000000000000000000000000000000000000000
--premine 0x228466F2C715CbEC05dEAbfAc040ce3619d7CF0B:0xD3C21BCECCEDA1000000
--premine 0xca48694ebcB2548dF5030372BE4dAad694ef174e:0xD3C21BCECCEDA1000000
--burn-contract 0:0x0000000000000000000000000000000000000000
EOL
)

# createGenesisConfig creates genesis configuration
createGenesisConfig() {
  local consensus_type="$1"
  local secrets="$2"
  shift 2
  echo "Generating $consensus_type Genesis file..."

  "$POLYGON_EDGE_BIN" genesis $CHAIN_CUSTOM_OPTIONS \
    --dir /data/genesis.json \
    --validators-path /data \
    --validators-prefix data- \
    --consensus $consensus_type \
    --bootnode "/dns4/node-1/tcp/1478/p2p/$(echo "$secrets" | jq -r '.[0] | .node_id')" \
    --bootnode "/dns4/node-2/tcp/1478/p2p/$(echo "$secrets" | jq -r '.[1] | .node_id')" \
    --bootnode "/dns4/node-3/tcp/1478/p2p/$(echo "$secrets" | jq -r '.[2] | .node_id')" \
    --bootnode "/dns4/node-4/tcp/1478/p2p/$(echo "$secrets" | jq -r '.[3] | .node_id')" \
    "$@"
}

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

                  createGenesisConfig "$2" "$secrets"
              fi
              ;;
          "polybft")
              echo "Generating PolyBFT secrets..."
              secrets=$("$POLYGON_EDGE_BIN" polybft-secrets init --insecure --num 4 --data-dir /data/data- --json)
              echo "Secrets have been successfully generated"

              rm -f /data/genesis.json

              proxyContractsAdmin=0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed

              createGenesisConfig "$2" "$secrets" \
                --reward-wallet 0xDEADBEEF:1000000 \
                --native-token-config "Polygon:MATIC:18:true:$(echo "$secrets" | jq -r '.[0] | .address')" \
                --proxy-contracts-admin ${proxyContractsAdmin}

              echo "Deploying stake manager..."
              "$POLYGON_EDGE_BIN" polybft stake-manager-deploy \
                --jsonrpc http://rootchain:8545 \
                --genesis /data/genesis.json \
                --proxy-contracts-admin ${proxyContractsAdmin} \
                --test

              stakeManagerAddr=$(cat /data/genesis.json | jq -r '.params.engine.polybft.bridge.stakeManagerAddr')
              stakeToken=$(cat /data/genesis.json | jq -r '.params.engine.polybft.bridge.stakeTokenAddr')

              "$POLYGON_EDGE_BIN" rootchain deploy \
                --stake-manager ${stakeManagerAddr} \
                --stake-token ${stakeToken} \
                --json-rpc http://rootchain:8545 \
                --genesis /data/genesis.json \
                --proxy-contracts-admin ${proxyContractsAdmin} \
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
                --finalize-genesis-set \
                --enable-staking \
                --genesis /data/genesis.json \
                --jsonrpc http://rootchain:8545
              ;;
      esac
      ;;
  "start-node-1")
    relayer_flag=""
    # Start relayer only when run in polybft
    if [ "$2" == "polybft" ]; then
      echo "Starting relayer..."
      relayer_flag="--relayer"
    fi

    "$POLYGON_EDGE_BIN" server \
      --data-dir /data/data-1 \
      --chain /data/genesis.json \
      --grpc-address 0.0.0.0:9632 \
      --libp2p 0.0.0.0:1478 \
      --jsonrpc 0.0.0.0:8545 \
      --prometheus 0.0.0.0:5001 \
      $relayer_flag
   ;;
   *)
      echo "Executing polygon-edge..."
      exec "$POLYGON_EDGE_BIN" "$@"
      ;;
esac
