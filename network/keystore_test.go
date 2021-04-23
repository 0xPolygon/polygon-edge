package network

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeystore_InMemory(t *testing.T) {
	// if we do not pass a data directory, the keys are
	// not stored
	key0, err := ReadLibp2pKey("")
	assert.NoError(t, err)

	key1, err := ReadLibp2pKey("")
	assert.NoError(t, err)

	assert.False(t, key0.Equals(key1))
}

func TestKeystore_Store(t *testing.T) {
	tmpDir, err := ioutil.TempDir("/tmp", "libp2p-keystore")
	assert.NoError(t, err)

	key0, err := ReadLibp2pKey(tmpDir)
	assert.NoError(t, err)

	key1, err := ReadLibp2pKey(tmpDir)
	assert.NoError(t, err)

	assert.True(t, key0.Equals(key1))
}
