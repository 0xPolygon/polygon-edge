package tests

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func ReadTestCase(t *testing.T, name string, target interface{}) {
	_, b, _, _ := runtime.Caller(0)
	d := path.Join(path.Dir(b))

	testsuiteDir := filepath.Join(filepath.Dir(d), "tests", "node_modules")
	if _, err := os.Stat(testsuiteDir); os.IsNotExist(err) {
		t.Skip("testcases not downloaded")
	}

	path := filepath.Join(testsuiteDir, "@ethersproject/testcases/testcases", name+".json.gz")

	f, err := os.Open(path)
	assert.NoError(t, err)

	zr, err := gzip.NewReader(f)
	assert.NoError(t, err)

	decoder := json.NewDecoder(zr)
	assert.NoError(t, decoder.Decode(target))
}
