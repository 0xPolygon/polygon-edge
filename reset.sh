rm -f genesis.json
rm -rf validator-1
rm -rf validator-2
rm -rf validator-3
rm -rf validator-4
VAL_ONE=$(./main ibft init --data-dir validator-1 | grep -o '\b16\w*')
VAL_TWO=$(./main ibft init --data-dir validator-2 | grep -o '\b16\w*')
VAL_THREE=$(./main ibft init --data-dir validator-3 | grep -o '\b16\w*')
VAL_FOUR=$(./main ibft init --data-dir validator-4 | grep -o '\b16\w*')
./main genesis --premine 0x7D5e3D5D63225832cbf723526aF2e4EA4833dB35:0x3635C9ADC5DEA00000000 --consensus ibft --ibft-validators-prefix-path validator- --bootnode /ip4/127.0.0.1/tcp/10001/p2p/$VAL_ONE --bootnode /ip4/127.0.0.1/tcp/20001/p2p/$VAL_TWO --bootnode /ip4/127.0.0.1/tcp/30001/p2p/$VAL_THREE --bootnode /ip4/127.0.0.1/tcp/40001/p2p/$VAL_FOUR
cp genesis.json validator-1
cp genesis.json validator-2
cp genesis.json validator-3
cp genesis.json validator-4