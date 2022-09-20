nodes=5
duration=25m

test:
	go test -v --race -shuffle=on -coverprofile=coverage.out -covermode=atomic ./...


property-tests:
	cd ./e2e && go test -v -run TestProperty -rapid.steps 10000

fuzz-e2e:
	cd ./e2e && go run ./cmd/main.go fuzz-run -nodes=$(nodes) -duration=$(duration)

unit-e2e:
	cd ./e2e && go test -v -run TestE2E

unit-fuzz:
	cd ./e2e && go test -timeout=20m -run TestFuzz

lintci:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.46.1

lint:
	@"$(GOPATH)/bin/golangci-lint" run --config ./.golangci.yml ./...


.PHONY: test e2e property-tests
