#!/bin/bash

# create genesis.json
go run . genesis --block-gas-limit 10000000 --epoch-size 10 \
    --premine 0x0000000000000000000000000000000000000000 \
    --validators /ip4/127.0.0.1/tcp/30301/p2p/16Uiu2HAkusnzShNLU1c1W6fJNgiVCSuyxaTbRyUiZB88Ajga1eKL:0x987AAE831094b6041EFd32fBEb0f64479EE67fD7:14532a927349da40f808a607a2603dffebac8ff42197fa968c56361bf95d6b2712fd2d4fe393d4d8029b86251ef8f521670fd142f73ef783799d9429cb26e79c0f9aae17efafb6f4c39e13a2d267100730b6443a87243cc0ec5bfe40cf50525a2a1e2b36e906e33a5e97d3e332e661a1de5ed0fcc30b815c8bc50eb9dfd80c67:255db8b00cbea84f6d83276ab8a5364d2832d96469f951078b263ba91f7d81490a5d29bdc3cb92df5930c9dd5203f70ee28640b4f0c99a6f107c3477257d6492 \
    --validators /ip4/127.0.0.1/tcp/30302/p2p/16Uiu2HAkxCAqHMEB7TbikFVmJ4A577Jgu2FAnGjF7WjugJSpFrXz:0x58Df72e2707d0478803097503b8433C3cb8Aa0AF:002ea094927c71ee9f5d70ad9e61cf84bed6020bdb61fb510baeece2a85915081391d2eb2f4a40b1bff3357cb5b00dfd1cf0701901a4686bb110a00f0e2cd4d02bc6f4d2c8ffc65390f48de32f7184bfe1dcd821fb873044850ace387660515a0d9a0e2a27bfa058d9246cf63035b69a20240c5135f369642a536d6828b2893d:0bb62ecec716b22f69cc47e52243bdcd9540c1df656f4194d5e7cc2264cfd9fb255c5bc8a8197f7026b7a60ab3564b9861c9b492526e16399875e6067c49dc8a \
    --validators /ip4/127.0.0.1/tcp/30303/p2p/16Uiu2HAmBKJGji8eY4zEFynLfgAXHeGGJhBkFexiZpTJJdAxdnaM:0x6dE8f86ECbb135238d759C20Cf9161197cb59f40:0de7773aead5ea0eaf03dc02d338c28ac497be368949e4a39f8cd0ad711ee4951c234fe0d538ca6bfb2a05349784bc06b3c6e8e4f893b87536d99ae7e339c1eb2c7d908f029f5176047b400cc259fac412584b21bbb76994faa0f593a957c6af180d0a27c7a437dfda7ff4afdf7aa8f3ef9a530ec44d0e034b23f019436706f5:1fa5e7802cac481838b2767af38f67a0feafb25c1b999f05443afa652c8cb9751b6134f376c02674571584a513c1f2780653c6ce931b80aa26bbe8866caeadef \
    --validators /ip4/127.0.0.1/tcp/30304/p2p/16Uiu2HAmKxH3tgswksn8uv55Ed5m6SK2dLaKv6qVdH1vTT6Ehezt:0x1DCEb48778846543d42C2580a90D3908d52AC3Bc:00622e56c971642591514236271e95190d7af4b9b818b6b290311d12d1a114ac07a17ba416fab3aff4d9ed9d26f48c715f65eeef8534ed83964ee027ba65ef4a1fdd473b42dbb787f7fc1060f0826e27722d6d0dab909123aa48e62bf0f98c3627c115f174d3577cdb82480635849b28eac5bf5e9668f6230018d5de98c6972a:1b6709dbc62f130b82af5ebd708bd3dd6da2036a4fdf684a03aadcfbc1b651701246fb28d8ee8e61bfb299d4223d7b167e45f227105f618e0220c184f11a7ea8 \
    --stake 0x987AAE831094b6041EFd32fBEb0f64479EE67fD7:123456789 \
    --premine 0x987AAE831094b6041EFd32fBEb0f64479EE67fD7:1234 \
    2>&1 | tee ./tmp/genesis.log

# deploy rootchain contracts
go run . rootchain deploy --test \
    2>&1 | tee ./tmp/rootchain-contracts.log

# fund validators on the rootchain
go run . rootchain fund --data-dir test-chain- --num 4

# run validators
go run . server --data-dir ./test-chain-1 --chain genesis.json --grpc-address :5001 --libp2p :30301 --jsonrpc :9545 --seal --log-level DEBUG \
    --relayer --num-block-confirmations 2 2>&1 | tee ./tmp/validator-1.log &
go run . server --data-dir ./test-chain-2 --chain genesis.json --grpc-address :5002 --libp2p :30302 --jsonrpc :10002 --seal --log-level DEBUG \
    --num-block-confirmations 2 2>&1 | tee ./tmp/validator-2.log &
go run . server --data-dir ./test-chain-3 --chain genesis.json --grpc-address :5003 --libp2p :30303 --jsonrpc :10003 --seal --log-level DEBUG \
    --num-block-confirmations 2 2>&1 | tee ./tmp/validator-3.log &
go run . server --data-dir ./test-chain-4 --chain genesis.json --grpc-address :5004 --libp2p :30304 --jsonrpc :10004 --seal --log-level DEBUG \
    --num-block-confirmations 2 2>&1 | tee ./tmp/validator-4.log
