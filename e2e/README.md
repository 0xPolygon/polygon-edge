# E2E tests

The implemented E2E tests start a local instance of polygon-edge.

As such, they require the binary 'polygon-edge' to be available in the $PATH variable.<br />
Typically, the actual directory added to the $PATH variable would be the `go/bin` folder.

## Step 1: Build the polygon-edge

```bash
go build -o $HOME/go/bin/polygon-edge ./main.go
```

## Step 2: Run the tests

Now that the polygon-edge binary is in the `go/bin` folder, the e2e test server is able to locate it when running tests.

## Manual checks if things are acting funny

### Check if the polygon-edge process is running

If you've stopped the tests abruptly, chances are the polygon-edge process is still running on your machine. <br/ >
In order for the tests to function normally, please kill the process.

### Check if the polygon-edge-* folders are present in /tmp/

While running, the e2e server stores data needed for running in the /tmp/polygon-edge* folders. <br />
To clean up these folders, simply run:

````bash
cd /tmp/
rm -rf polygon-edge-*
````

### Clean the golang test cache

Golang caches test results, and may not even run a test, causing tests to fail on some machines and work on others.
````bash
go clean -testcache
````

This command cleans the test cache, and should be added to the runtime config.

Another way to disable test caching altogether is to add the following flag when running go test `-count=1`:
````bash
go test ./... -count=1
````

## Note

### constants

Constant values that used in some e2e tests are defined in `e2e/const.go`.
Mock contract are pre-compiled and the result is stored in `const.go` in order to avoid dependencies of solc command (solidity compiler).

You can get the byte code from original program by following command.

```shell
$ solc --bin e2e/sample.sol
```

Currently you need to build with version 0.5.x compiler. You can check the compiler version by `solc --version`.

```shell
$ solc --version
solc, the solidity compiler commandline interface
Version: 0.5.17+commit.d19bba13.Darwin.appleclang
```
