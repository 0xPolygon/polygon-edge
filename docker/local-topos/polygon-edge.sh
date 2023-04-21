#!/bin/sh

set -e

POLYGON_EDGE_BIN=./polygon-edge
CONTRACTS_PATH=/contracts
GENESIS_PATH=/data/genesis.json
CHAIN_ID="${CHAIN_ID:-100}" # 100 is Edge's default value
NUMBER_OF_NODES="${NUMBER_OF_NODES:-4}" # Number of subnet nodes in the consensus
BOOTNODE_DOMAIN_NAME="${BOOTNODE_DOMAIN_NAME:-node-1}"
CHAIN_CUSTOM_OPTIONS=$(tr "\n" " " << EOL
--block-gas-limit 10000000
--epoch-size 10
--chain-id $CHAIN_ID
--name polygon-edge-docker
--premine 0x228466F2C715CbEC05dEAbfAc040ce3619d7CF0B:0xD3C21BCECCEDA1000000
--premine 0xca48694ebcB2548dF5030372BE4dAad694ef174e:0xD3C21BCECCEDA1000000
--premine 0x4AAb25B4fAd0Beaac466050f3A7142A502f4Cf0a:1000000000000000000000
EOL
)

case "$1" in

    "init")
        data_dir="/data/data-"
        if [ "$NUMBER_OF_NODES" -eq "1" ]; then
            data_dir="/data/data-1"
        fi

        case "$2" in 

            "ibft")
                if [ -f "$GENESIS_PATH" ]; then
                    echo "Secrets have already been generated."
                else
                    echo "Generating secrets..."
                    secrets=$("$POLYGON_EDGE_BIN" secrets init --insecure \
                    --num "$NUMBER_OF_NODES" --data-dir "$data_dir" --json)
                    chmod -R 755 /data # TOPOS: To make secret readable from the sequencer
                    echo "Secrets have been successfully generated"

                    BOOTNODE_ID=$(echo $secrets | jq -r '.[0] | .node_id')
                    BOOTNODE_ADDRESS=$(echo $secrets | jq -r '.[0] | .address')

                    echo "Generating IBFT Genesis file..."
                    cd /data && /polygon-edge/polygon-edge genesis $CHAIN_CUSTOM_OPTIONS \
                      --dir genesis.json \
                      --consensus ibft \
                      --ibft-validators-prefix-path data- \
                      --validator-set-size=$NUMBER_OF_NODES \
                      --bootnode /dns4/"$BOOTNODE_DOMAIN_NAME"/tcp/1478/p2p/$BOOTNODE_ID \
                      --premine=$BOOTNODE_ADDRESS:1000000000000000000000 \
                    && cd /polygon-edge
                fi    
            ;;

            "polybft")
                if [ -f "$GENESIS_PATH" ]; then
                    echo "Secrets have already been generated."
                else
                    echo "Generating PolyBFT secrets..."
                    secrets=$("$POLYGON_EDGE_BIN" polybft-secrets init --insecure \
                    --num "$NUMBER_OF_NODES" --data-dir "$data_dir" --json)
                    chmod -R 755 /data # TOPOS: To make secret readable from the sequencer
                    echo "Secrets have been successfully generated"

                    BOOTNODE_ID=$(echo $secrets | jq -r '.[0] | .node_id')
                    BOOTNODE_ADDRESS=$(echo $secrets | jq -r '.[0] | .address')

                    echo "Generating manifest..."
                    "$POLYGON_EDGE_BIN" manifest --path /data/manifest.json --validators-path /data \
                    --validators-prefix data- --chain-id "$CHAIN_ID"

                    echo "Generating PolyBFT Genesis file..."
                    "$POLYGON_EDGE_BIN" genesis $CHAIN_CUSTOM_OPTIONS \
                      --dir "$GENESIS_PATH" \
                      --consensus polybft \
                      --manifest /data/manifest.json \
                      --validator-set-size=$NUMBER_OF_NODES \
                      --bootnode /dns4/"$BOOTNODE_DOMAIN_NAME"/tcp/1478/p2p/$BOOTNODE_ID
                fi
            ;;
        esac

        echo "Predeploying ConstAddressDeployer contract..."
        CONST_ADDRESS_DEPLOYER_ADDRESS=0x0000000000000000000000000000000000001110
        "$POLYGON_EDGE_BIN" genesis predeploy \
          --chain "$GENESIS_PATH" \
          --artifacts-path "$CONTRACTS_PATH"/topos-core/ConstAddressDeployer.sol/ConstAddressDeployer.json \
          --predeploy-address "$CONST_ADDRESS_DEPLOYER_ADDRESS" \
          2>&1 >/dev/null && echo "ConstAddressDeployer has been successfully predeployed!" \
          || echo "Predeployment of ConstAddressDeployer failed with error code $?"
    ;;

    "standalone-test")
        echo "Cleaning up previous execution..."
        rm -rf genesis.json data-1
        echo "Generating node secrets..."
        NODE_ID=`$POLYGON_EDGE_BIN secrets init --insecure --num 1 --data-dir data-1 | grep Node | awk '{ print $4}'`
        echo "Boot node id: " $NODE_ID
        echo "Validator private key:" `cat data-1/consensus/validator.key`
        echo "Generating genesis script..."
        "$POLYGON_EDGE_BIN" genesis --dir genesis.json \
         --consensus ibft \
         --ibft-validators-prefix-path data- \
         --validator-set-size=1 \
         --premine=0x4AAb25B4fAd0Beaac466050f3A7142A502f4Cf0a:1000000000000000000000 \
         --bootnode /ip4/127.0.0.1/tcp/10001/p2p/$NODE_ID

        echo "Executing polygon-edge standalone node..."
        exec "$POLYGON_EDGE_BIN" server --data-dir ./data-1 --chain genesis.json
    ;;    

    *)
        echo "Executing polygon-edge..."
        exec "$POLYGON_EDGE_BIN" "$@"
    ;;

esac
