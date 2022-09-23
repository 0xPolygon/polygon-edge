
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
	golangci-lint run -E whitespace -E wsl -E wastedassign -E unconvert -E tparallel -E thelper -E stylecheck -E prealloc \
	-E predeclared -E nlreturn -E misspell -E makezero -E lll -E importas -E gosec -E  gofmt -E goconst \
	-E forcetypeassert -E dogsled -E dupl -E errname -E errorlint -E nolintlint --timeout 2m

.PHONY: generate-bsd-licenses
generate-bsd-licenses:
	./generate_dependency_licenses.sh BSD-3-Clause,BSD-2-Clause > ./licenses/bsd_licenses.json

.PHONY: test
test:
	go build -o artifacts/polygon-edge .
	$(eval export PATH=$(shell pwd)/artifacts:$(PATH))
	go test -timeout 28m ./...

.PHONY: run-local
run-local:
	docker-compose -f ./docker/local/docker-compose.yml up -d --build

.PHONY: stop-local
stop-local:
	docker-compose -f ./docker/local/docker-compose.yml stop

