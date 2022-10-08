
.PHONY: download-spec-tests
download-spec-tests:
	git submodule init
	git submodule update

.PHONY: bindata
bindata:
	go-bindata -pkg chain -o ./chain/chain_bindata.go ./chain/chains

.PHONY: protoc
protoc:
	protoc --go_out=. --go-grpc_out=. ./server/proto/*.proto
	protoc --go_out=. --go-grpc_out=. ./protocol/proto/*.proto
	protoc --go_out=. --go-grpc_out=. ./network/proto/*.proto
	protoc --go_out=. --go-grpc_out=. ./txpool/proto/*.proto
	protoc --go_out=. --go-grpc_out=. ./consensus/ibft/**/*.proto

.PHONY: build
build:
	$(eval LATEST_VERSION = $(shell git describe --tags --abbrev=0))
	$(eval COMMIT_HASH = $(shell git rev-parse HEAD))
	$(eval BRANCH = $(shell git rev-parse --abbrev-ref HEAD | tr -d '\040\011\012\015\n'))
	$(eval TIME = $(shell date))
	go build -o polygon-edge -ldflags="\
    	-X 'github.com/0xPolygon/polygon-edge/versioning.Version=$(LATEST_VERSION)' \
		-X 'github.com/0xPolygon/polygon-edge/versioning.Commit=$(COMMIT_HASH)'\
		-X 'github.com/0xPolygon/polygon-edge/versioning.Branch=$(BRANCH)'\
		-X 'github.com/0xPolygon/polygon-edge/versioning.BuildTime=$(TIME)'" \
	main.go

.PHONY: lint
lint:
	golangci-lint run --config .golangci.yml

.PHONY: generate-bsd-licenses
generate-bsd-licenses:
	./generate_dependency_licenses.sh BSD-3-Clause,BSD-2-Clause > ./licenses/bsd_licenses.json

.PHONY: test
test:
	go test -timeout=20m `go list ./... | grep -v e2e`

.PHONY: test-e2e
test-e2e:
    # We need to build the binary with the race flag enabled
    # because it will get picked up and run during e2e tests
    # and the e2e tests should error out if any kind of race is found
	go build -race -o artifacts/polygon-edge .
	env EDGE_BINARY=${PWD}/artifacts/polygon-edge go test -v -timeout=30m ./e2e/...

.PHONY: run-local
run-local:
	docker-compose -f ./docker/local/docker-compose.yml up -d --build

.PHONY: stop-local
stop-local:
	docker-compose -f ./docker/local/docker-compose.yml stop

