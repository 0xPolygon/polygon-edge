package keystore

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalKeystore(t *testing.T) {
	dir, err := ioutil.TempDir("/tmp", "test-localkeystore-")
	assert.NoError(t, err)

	defer os.RemoveAll(dir)

	keystore := LocalKeystore{
		Path: filepath.Join(dir, "key"),
	}

	_, err = keystore.Get()
	assert.NoError(t, err)
}
