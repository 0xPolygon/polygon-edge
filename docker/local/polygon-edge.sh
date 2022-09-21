#!/bin/sh

set -e

POLYGON_EDGE_BIN=./polygon-edge

case "$1" in

   "init")
      echo "Generating secrets..."
      node1id=$("$POLYGON_EDGE_BIN" secrets init --data-dir /data1 | grep Node | awk -F ' ' '{print $4}')
      node2id=$("$POLYGON_EDGE_BIN" secrets init --data-dir /data2 | grep Node | awk -F ' ' '{print $4}')
      "$POLYGON_EDGE_BIN" secrets init --data-dir /data3 | grep Node | awk -F ' ' '{print $4}'
      "$POLYGON_EDGE_BIN" secrets init --data-dir /data4 | grep Node | awk -F ' ' '{print $4}'
      echo "Secrets have been successfully generated"

      echo "Generating genesis file..."
      "$POLYGON_EDGE_BIN" genesis --dir /genesis/genesis.json --consensus ibft --ibft-validators-prefix-path data --bootnode /ip4/127.0.0.1/tcp/10001/p2p/"$node1id" --bootnode /ip4/127.0.0.1/tcp/20001/p2p/"$node2id"
      echo "Genesis file has been successfully generated"
      ;;

   *)
      echo "Executing polygon-edge..."
      exec "$POLYGON_EDGE_BIN" "$@"
      ;;

esac
