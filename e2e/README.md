
# E2E tests

Note that these tests exec Polygon-sdk. Thus, if they happen to fatally fail, the process might be running on the background and it would require manual shutdown.

It requires to have the binary 'polygon-sdk' available on Path.

```
$ go build -o $HOME/go/bin/polygon-sdk ./main.go
```
