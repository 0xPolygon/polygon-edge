# E2E tests

The implemented E2E tests start a local instance of polygon-sdk.

As such, they require the binary 'polygon-sdk' to be available in the $PATH variable.<br />
Typically, the actual directory added to the $PATH variable would be the `go/bin` folder.

## Step 1: Build the polygon-sdk

```bash
go build -o $HOME/go/bin/polygon-sdk ./main.go
```

## Step 2: Run the tests

Now that the polygon-sdk binary is in the `go/bin` folder, the e2e test server is able to locate it when running tests.

## Manual checks if things are acting funny

### Check if the polygon-sdk process is running

If you've stopped the tests abruptly, chances are the polygon-sdk process is still running on your machine. <br/ >
In order for the tests to function normally, please kill the process.

### Check if the polygon-sdk-* folders are present in /tmp/

While running, the e2e server stores data needed for running in the /tmp/polygon-sdk* folders. <br />
To clean up these folders, simply run:

````bash
cd /tmp/
rm -rf polygon-sdk-*
````