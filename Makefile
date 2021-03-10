
GOX=$(shell which gox > /dev/null 2>&1 ; echo $$? )
PACKR=$(shell which packr > /dev/null 2>&1 ; echo $$? )

.PHONY: build
build: static-assets
	@echo "--> Build"
ifeq (1,${GOX})
	@echo "--> Error: Install gox to build minimal. See https://github.com/mitchellh/gox"
	exit 1
endif
	@sh -c ./scripts/build.sh

.PHONY: static-assets
static-assets:
	@echo "--> Generating static assets for the json chains"

ifeq (1,${PACKR})
	@echo "--> Error: Install packr to generate static assets. See https://github.com/gobuffalo/packr"
	exit 1
endif
	@packr -i ./chain

.PHONY: clean
clean:
	@echo "--> Cleaning build artifacts"
ifeq (1,${PACKR})
	@echo "--> Error: Install packr to generate static assets. See https://github.com/gobuffalo/packr"
	exit 1
endif
	@packr clean

.PHONY: bindata
bindata:
	go-bindata -pkg chain -o ./chain/chain_bindata.go ./chain/chains

.PHONY: protoc
protoc:
	protoc --go_out=plugins=grpc:. ./minimal/proto/*.proto

.PHONY: clean-ibft-dir
clean-ibft-dir:
	rm -rf ./.ibft1/blockchain ./.ibft1/consensus ./.ibft1/network ./.ibft1/trie
	rm -rf ./.ibft2/blockchain ./.ibft2/consensus ./.ibft2/network ./.ibft2/trie
	rm -rf ./.ibft3/blockchain ./.ibft3/consensus ./.ibft3/network ./.ibft3/trie
	rm -rf ./.ibft4/blockchain ./.ibft4/consensus ./.ibft4/network ./.ibft4/trie