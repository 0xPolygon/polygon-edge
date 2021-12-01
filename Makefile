
.PHONY: download-spec-tests
download-spec-tests:
	git submodule init
	git submodule update

.PHONY: bindata
bindata:
	go-bindata -pkg chain -o ./chain/chain_bindata.go ./chain/chains

.PHONY: protoc
protoc:
	protoc --go_out=. --go-grpc_out=. ./minimal/proto/*.proto
	protoc --go_out=. --go-grpc_out=. ./protocol/proto/*.proto
	protoc --go_out=. --go-grpc_out=. ./network/proto/test/*.proto
	protoc --go_out=. --go-grpc_out=. ./network/proto/*.proto
	protoc --go_out=. --go-grpc_out=. ./txpool/proto/*.proto
	protoc --go_out=. --go-grpc_out=. ./consensus/ibft/proto/*.proto

.PHONY: ibft-init
ibft-init:
	rm -rf genesis.json test-chain-*
	go run main.go secrets init --data-dir test-chain-1
	go run main.go secrets init --data-dir test-chain-2
	go run main.go secrets init --data-dir test-chain-3
	go run main.go secrets init --data-dir test-chain-4

.PHONY: run
run:
	go run ./main.go server --data-dir ./test-chain-${N} --grpc 172.31.25.132:1${N}000 --libp2p 172.31.25.132:1${N}001 --jsonrpc 172.31.25.132:1${N}002 --seal --nat 18.188.213.236 --chain ./genesis.json --log-level INFO | tee log${N}.txt

.PHONY: run-noseal
run-noseal:
	go run ./main.go server --data-dir ./test-chain-${N} --grpc :1${N}000 --libp2p :1${N}001 --jsonrpc :1${N}002 --chain ./genesis.json --log-level INFO | tee log${N}.txt

.PHONY: clean
clean:
	rm -rf test-chain-1/blockchain/* test-chain-1/consensus/metadata test-chain-1/consensus/snapshots test-chain-1/trie/*
	rm -rf test-chain-2/blockchain/* test-chain-2/consensus/metadata test-chain-2/consensus/snapshots test-chain-2/trie/*
	rm -rf test-chain-3/blockchain/* test-chain-3/consensus/metadata test-chain-3/consensus/snapshots test-chain-3/trie/*
	rm -rf test-chain-4/blockchain/* test-chain-4/consensus/metadata test-chain-4/consensus/snapshots test-chain-4/trie/*
	rm -rf test-chain-4/blockchain/* test-chain-5/consensus/metadata test-chain-5/consensus/snapshots test-chain-5/trie/*
	rm -rf test-chain-4/blockchain/* test-chain-6/consensus/metadata test-chain-6/consensus/snapshots test-chain-6/trie/*
	rm -rf test-chain-4/blockchain/* test-chain-7/consensus/metadata test-chain-7/consensus/snapshots test-chain-7/trie/*

.PHONY: clean-one
clean-one:
	rm -rf test-chain-${N}/blockchain/* test-chain-${N}/consensus/metadata test-chain-${N}/consensus/snapshots test-chain-${N}/trie/*

.PHONY: dev-init
dev-init:
	rm -rf genesis.json test-chain*
	go run ./main.go genesis --consensus dev --premine 0xba8cae91c858120e9699f9b9936418c6bd3eccc1 --premine 0x9aFc51c7867f369371F238674ee4459556c5D2b5

.PHONY: dev-start
dev-start:
	go run ./main.go server --data-dir ./test-chain --grpc :10000 --libp2p :10001 --jsonrpc :10002 --seal --chain ./genesis.json --log-level DEBUG

.PHONY: test-all
test-all:
	go build -o main main.go && mv ./main /usr/local/bin/polygon-sdk
	go clean --testcache
	go test ./...

.PHONY: deploy-bridge-contracts
deploy-bridge-contracts:
	cb-sol-cli deploy --all --chainId 102 \
	--url http://localhost:11002 \
	--privateKey 2cd1661e5460e6200b7a7464e2d2015cd0613cc59b365ceb0d6ff8ad813ab6fb \
	--gasPrice 10 \
	--relayerThreshold 2 --relayers 0x1E406f2c180Fab7E23d4e0b60f5d6F49bEF1590F,0x55107bb5395A3E3F56fFF2E7e0a758e15fD6A637,0xC6f5CB1AaAA628891CDC01568C73233AbD4C0d6C,0xEe748358Ec683D331cea39D3014B748164F5abe2
