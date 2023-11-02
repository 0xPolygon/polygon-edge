# Regenesis procedure

This document outlines step necessary to perform a regenesis data migration.

## Steps

1. Create cluster

    ```bash
    scripts/cluster ibft
    ```

2. Check balance

    ```bash
    curl -s -X POST --data '{"jsonrpc":"2.0", "method":"eth_getBalance", "params":["0x85da99c8a7c2c95964c8efd687e95e632fc533d6", "latest"], "id":1}' http://localhost:10002

    {"jsonrpc":"2.0","id":1,"result":"0x3635c9adc5dea00000"}
    ```

3. Get trie root

    ```bash
    ./polygon-edge regenesis getroot --rpc "http://localhost:10002"

    [Trie copy SUCCESS]
    state root 0xf5ef1a28c82226effb90f4465180ec3469226747818579673f4be929f1cd8663 for block 38
    ```

4. Make trie snapshot

    ```bash
    ./polygon-edge regenesis --target-path ./trie_new --stateRoot 0xf5ef1a28c82226effb90f4465180ec3469226747818579673f4be929f1cd8663  --source-path ./test-chain-1/trie 

    [Trie copy SUCCESS]

    ```

5. Remove old chain data

    ```bash
    rm -rf test-chain-*
    ```

6. Create new validators

    ```bash
    ./polygon-edge polybft-secrets --insecure --data-dir test-chain- --num 4

    [WARNING: INSECURE LOCAL SECRETS - SHOULD NOT BE RUN IN PRODUCTION]

    [SECRETS INIT]
    Public key (address. = 0x467CaA6185461E4c518597dCE7DE497Fb98a5680
    BLS Public key       = 197059fdc3a78bd4001d802481c5ff1d84870c0b37aef851c83522323bd80f6429751ddae63cf38d180a8d45a5b4bf5519f380d60d6eedf9ccd22c3f95fc5e3a1193155af6ff3ab9e2aea5beab3f52e5e364d2cb410d6108c92f9a8375aac73110b8526407691c7e92e5cfab984e9011202b0606dc2be942808554b848cce67b
    Node ID              = 16Uiu2HAmTN2YAviWyyG4A56Zz8gsVJhmksdojymS4pk54SZqVbGV

    [WARNING: INSECURE LOCAL SECRETS - SHOULD NOT BE RUN IN PRODUCTION]

    [SECRETS INIT]
    Public key (address. = 0x33177bBAebcB20F8545864a83b9b6EE334e4f94D
    BLS Public key       = 12d82d172646703d298453cef6f4415ceab2052267d9ec300fd4742fa46fd8db0168b5e98372ae3efd52d647f9b356043163fc4f76182f1ef685faf5e53b1dba15b7d0d9cb9b7868592e02179255775618dbacafd384cffba95b647a5d84de9a27995bb8cc0766194f370ad5b274d1a53b9b8ab21a3dee2f4ee4f177f63fb1f0
    Node ID              = 16Uiu2HAkvtXkr1Hsct3UGn19ULNb7jgzPvEPu6Fpdakco8P45648

    [WARNING: INSECURE LOCAL SECRETS - SHOULD NOT BE RUN IN PRODUCTION]

    [SECRETS INIT]
    Public key (address. = 0xf308dF858dA25c2e40485FfA2c037D98105FD254
    BLS Public key       = 220efa87e71744f44e286230cf4ac95a419abdd6d33d1a578fee21387b841fa32184dd132c077adb8f46db0eb66333672e36b5f363778030475672defce0b5622bb0974424dd7babfdf722fe9eab75ab6e5e34e2ecfcea89420f757bf8adfeb7020f0f961ca946cdb2dde40413c5c0d48aa9f13182ec35b58c4052de466c0e25
    Node ID              = 16Uiu2HAmJqYRtWGbepPjQMgWfXnFEkXuX4GKH8AdD1voTwHpKuFa

    [WARNING: INSECURE LOCAL SECRETS - SHOULD NOT BE RUN IN PRODUCTION]

    [SECRETS INIT]
    Public key (address. = 0x7eC6b7fc98D988472AD1AC0cFcaC6DA993d865B0
    BLS Public key       = 2b8188f4bc99a19d5476fc97d14e17231cfb80b205a9fc45261725edefb0195209d972e9c1ad7d3c52b8fa129637738a88203c92fe8aa70fd0998d00f6251cb403ee077ddb4192fac270fe321468fe209308ea7597288e2505ed819f551ed00510c61f3da60f6a83d6017cbaeb7590c44bd354415178bbb160701b12a72a35a6
    Node ID              = 16Uiu2HAmEuYYyzQKpyVr2HVCG8Gqx5e5DLCi8LWY4TkFYvHYcWAq

    ```

7. Generate genesis file

    ```bash
    ./polygon-edge genesis --consensus polybft \
    --block-gas-limit 10000000 \
    --proxy-contracts-admin 0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed \
    --epoch-size 10 --trieroot 0xf5ef1a28c82226effb90f4465180ec3469226747818579673f4be929f1cd8663

    [GENESIS SUCCESS]
    **** POLYBFT CONSENSUS PROTOCOL IS IN EXPERIMENTAL PHASE AND IS NOT FULLY PRODUCTION READY. YOU ARE USING IT AT YOUR OWN RISK. ****
    Genesis written to ./genesis.json

    ```

8. Try to start a new v0.7 chain

    ```bash
    ./polygon-edge server --data-dir ./test-chain-1 --chain genesis.json --grpc-address :10000 --libp2p :30301 --jsonrpc :10002 --seal --log-level DEBUG &
    ./polygon-edge server --data-dir ./test-chain-2 --chain genesis.json --grpc-address :20000 --libp2p :30302 --jsonrpc :20002 --seal --log-level DEBUG &
    ./polygon-edge server --data-dir ./test-chain-3 --chain genesis.json --grpc-address :30000 --libp2p :30303 --jsonrpc :30002 --seal --log-level DEBUG &
    ./polygon-edge server --data-dir ./test-chain-4 --chain genesis.json --grpc-address :40000 --libp2p :30304 --jsonrpc :40002 --seal --log-level DEBUG &
    wait

    [1] 2615
    [2] 2616
    [3] 2617
    [4] 2618
    2023-03-15T11:02:25.149+0400 [INFO]  polygon.server: Data dir: path=./test-chain-1
    2023-03-15T11:02:25.149+0400 [DEBUG] polygon.server: DataDog profiler disabled, set DD_PROFILING_ENABLED env var to enable it.
    2023-03-15T11:02:25.233+0400 [INFO]  polygon.server: Data dir: path=./test-chain-3
    2023-03-15T11:02:25.251+0400 [DEBUG] polygon.server: DataDog profiler disabled, set DD_PROFILING_ENABLED env var to enable it.
    invalid initial state root
    [1]    exit 1     ./polygon-edge server --data-dir ./test-chain-1 --chain genesis.json  :10000 
    2023-03-15T11:02:25.299+0400 [INFO]  polygon.server: Data dir: path=./test-chain-2
    2023-03-15T11:02:25.302+0400 [DEBUG] polygon.server: DataDog profiler disabled, set DD_PROFILING_ENABLED env var to enable it.
    2023-03-15T11:02:25.396+0400 [INFO]  polygon.server: Data dir: path=./test-chain-4
    2023-03-15T11:02:25.413+0400 [DEBUG] polygon.server: DataDog profiler disabled, set DD_PROFILING_ENABLED env var to enable it.
    invalid initial state root
    [3]  - exit 1     ./polygon-edge server --data-dir ./test-chain-3 --chain genesis.json  :30000 
    invalid initial state root
    [2]  - exit 1     ./polygon-edge server --data-dir ./test-chain-2 --chain genesis.json  :20000 
    invalid initial state root
    [4]  + exit 1     ./polygon-edge server --data-dir ./test-chain-4 --chain genesis.json  :40000 
    ```

    It fails, because we haven't provided trie database with correct state trie.

9. Copy snapshot trie to our data directory

    ```bash
    rm -rf ./test-chain-1/trie
    rm -rf ./test-chain-2/trie
    rm -rf ./test-chain-3/trie
    rm -rf ./test-chain-4/trie
    cp -fR ./trie_new/ ./test-chain-1/trie/
    cp -fR ./trie_new/ ./test-chain-2/trie/
    cp -fR ./trie_new/ ./test-chain-3/trie/
    cp -fR ./trie_new/ ./test-chain-4/trie/
    ```

10. Run chain again

    ```bash
    ./polygon-edge server --data-dir ./test-chain-1 --chain genesis.json --grpc-address :10000 --libp2p :30301 --jsonrpc :10002 --seal --log-level DEBUG &
    ./polygon-edge server --data-dir ./test-chain-2 --chain genesis.json --grpc-address :20000 --libp2p :30302 --jsonrpc :20002 --seal --log-level DEBUG &
    ./polygon-edge server --data-dir ./test-chain-3 --chain genesis.json --grpc-address :30000 --libp2p :30303 --jsonrpc :30002 --seal --log-level DEBUG &
    ./polygon-edge server --data-dir ./test-chain-4 --chain genesis.json --grpc-address :40000 --libp2p :30304 --jsonrpc :40002 --seal --log-level DEBUG &
    wait

    [1] 2721
    [2] 2722
    [3] 2723
    [4] 2724
    2023-03-15T11:09:41.481+0400 [INFO]  polygon.server: Data dir: path=./test-chain-2
    2023-03-15T11:09:41.481+0400 [DEBUG] polygon.server: DataDog profiler disabled, set DD_PROFILING_ENABLED env var to enable it.
    2023-03-15T11:09:41.597+0400 [INFO]  polygon.server: Data dir: path=./test-chain-1
    2023-03-15T11:09:41.597+0400 [DEBUG] polygon.server: DataDog profiler disabled, set DD_PROFILING_ENABLED env var to enable it.
    2023-03-15T11:09:41.609+0400 [WARN]  polygon: Initial state root checked and correct
    2023-03-15T11:09:41.661+0400 [INFO]  polygon.server: Data dir: path=./test-chain-4
    2023-03-15T11:09:41.661+0400 [DEBUG] polygon.server: DataDog profiler disabled, set DD_PROFILING_ENABLED env var to enable it.
    2023-03-15T11:09:41.725+0400 [INFO]  polygon.server: Data dir: path=./test-chain-3
    2023-03-15T11:09:41.725+0400 [DEBUG] polygon.server: DataDog profiler disabled, set DD_PROFILING_ENABLED env var to enable it.
    2023-03-15T11:09:41.844+0400 [INFO]  polygon.blockchain: genesis: hash=0x627b70a8abc294324808d9820015faa5e0616afca5fdc75528b03b703a461acd
    2023-03-15T11:09:41.844+0400 [INFO]  polygon.server.polybft: initializing polybft...
    2023-03-15T11:09:41.951+0400 [WARN]  polygon: Initial state root checked and correct
    2023-03-15T11:09:42.101+0400 [WARN]  polygon: Initial state root checked and correct
    2023-03-15T11:09:42.254+0400 [WARN]  polygon: Initial state root checked and correct
    2023-03-15T11:09:42.445+0400 [INFO]  polygon.blockchain: genesis: hash=0x627b70a8abc294324808d9820015faa5e0616afca5fdc75528b03b703a461acd
    2023-03-15T11:09:42.445+0400 [INFO]  polygon.server.polybft: initializing polybft...
    2023-03-15T11:09:42.462+0400 [INFO]  polygon.server.polybft.consensus_runtime: restartEpoch: block number=0 epoch=1 validators=4 firstBlockInEpoch=1
    ...
    ```

11. Check that balance of account on v0.6 is not 0

    ```bash
    curl -s -X POST --data '{"jsonrpc":"2.0", "method":"eth_getBalance", "params":["0x85da99c8a7c2c95964c8efd687e95e632fc533d6", "latest"], "id":1}' http://localhost:10002

    {"jsonrpc":"2.0","id":1,"result":"0x3635c9adc5dea00000"}% 
    ```
