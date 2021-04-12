
.PHONY: download-spec-tests
download-spec-tests:
	git submodule init
	git submodule update

.PHONY: build
build: static-assets
	./scripts/build.sh

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
	protoc --go_out=. --go-grpc_out=. ./consensus/ibft2/proto/*.proto

.PHONY: clean-ibft-dir
clean-ibft-dir:
	rm -rf ./.ibft1/blockchain ./.ibft1/consensus ./.ibft1/network ./.ibft1/trie
	rm -rf ./.ibft2/blockchain ./.ibft2/consensus ./.ibft2/network ./.ibft2/trie
	rm -rf ./.ibft3/blockchain ./.ibft3/consensus ./.ibft3/network ./.ibft3/trie
	rm -rf ./.ibft4/blockchain ./.ibft4/consensus ./.ibft4/network ./.ibft4/trie
