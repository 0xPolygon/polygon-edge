#!/bin/sh

set -e

BLADE_BIN=./blade
CHAIN_CUSTOM_OPTIONS=$(tr "\n" " " << EOL
--block-gas-limit 10000000
--epoch-size 10
--chain-id 51001
--name Blade
--premine 0x0000000000000000000000000000000000000000
--premine 0x228466F2C715CbEC05dEAbfAc040ce3619d7CF0B:0xD3C21BCECCEDA1000000
--premine 0xca48694ebcB2548dF5030372BE4dAad694ef174e:0xD3C21BCECCEDA1000000
EOL
)

# createGenesisConfig creates genesis configuration
createGenesisConfig() {
  local consensus_type="$1"
  local secrets="$2"
  shift 2
  echo "Generating $consensus_type Genesis file..."

  "$BLADE_BIN" genesis $CHAIN_CUSTOM_OPTIONS \
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
          "polybft")
              echo "Generating PolyBFT secrets..."
              secrets=$("$BLADE_BIN" secrets init --insecure --num 4 --data-dir /data/data- --json)
              echo "Secrets have been successfully generated"

              rm -f /data/genesis.json

              proxyContractsAdmin=0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed

              createGenesisConfig "$2" "$secrets" \
                --reward-wallet 0xDEADBEEF:1000000 \
                --native-token-config "Polygon:MATIC:18" \
                --blade-admin $(echo "$secrets" | jq -r '.[0] | .address') \
                --proxy-contracts-admin ${proxyContractsAdmin}

              "$BLADE_BIN" bridge deploy \
                --json-rpc http://rootchain:8545 \
                --genesis /data/genesis.json \
                --proxy-contracts-admin ${proxyContractsAdmin} \
                --test

              addresses="$(echo "$secrets" | jq -r '.[0] | .address'),$(echo "$secrets" | jq -r '.[1] | .address'),$(echo "$secrets" | jq -r '.[2] | .address'),$(echo "$secrets" | jq -r '.[3] | .address')"

              "$BLADE_BIN" bridge fund \
                --json-rpc http://rootchain:8545 \
                --addresses ${addresses} \
                --amounts 1000000000000000000000000,1000000000000000000000000,1000000000000000000000000,1000000000000000000000000

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

    "$BLADE_BIN" server \
      --data-dir /data/data-1 \
      --chain /data/genesis.json \
      --grpc-address 0.0.0.0:9632 \
      --libp2p 0.0.0.0:1478 \
      --jsonrpc 0.0.0.0:8545 \
      --prometheus 0.0.0.0:5001 \
      $relayer_flag
   ;;
   *)
      echo "Executing blade..."
      exec "$BLADE_BIN" "$@"
      ;;
esac
