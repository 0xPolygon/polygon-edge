
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


.PHONY: lint
lint:
	golangci-lint run -E whitespace -E wsl -E wastedassign -E unconvert -E tparallel -E thelper -E stylecheck -E prealloc \
	-E predeclared -E nlreturn -E misspell -E makezero -E lll -E importas -E ifshort -E gosec -E  gofmt -E goconst \
	-E forcetypeassert -E dogsled -E dupl -E errname -E errorlint -E nolintlint --timeout 2m

.PHONY: test
test:
	CGO_ENABLED=1 go build -race -a -o /tmp/polygon-edge .
	export "PATH=/tmp:$$PATH"; go clean -testcache && go test -timeout 28m ./...