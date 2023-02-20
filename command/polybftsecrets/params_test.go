package polybftsecrets

import (
	"os"
	"path"
	"testing"

	"github.com/0xPolygon/polygon-edge/secrets/helper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test initKeys
func Test_initKeys(t *testing.T) {
	// Creates test directory
	dir, err := os.MkdirTemp("", "test")
	defer os.RemoveAll(dir)

	sm, err := helper.SetupLocalSecretsManager(dir)
	require.NoError(t, err)

	ip := &initParams{
		generatesAccount: false,
		generatesNetwork: false,
		chainID:          1,
	}

	_, err = ip.initKeys(sm)
	require.NoError(t, err)

	assert.False(t, fileExists(path.Join(dir, "consensus/validator.key")))
	assert.False(t, fileExists(path.Join(dir, "consensus/validator-bls.key")))
	assert.False(t, fileExists(path.Join(dir, "libp2p/libp2p.key")))
	assert.False(t, fileExists(path.Join(dir, "consensus/validator.sig")))

	ip.generatesAccount = true
	res, err := ip.initKeys(sm)
	require.NoError(t, err)
	assert.Len(t, res, 3)

	assert.True(t, fileExists(path.Join(dir, "consensus/validator.key")))
	assert.True(t, fileExists(path.Join(dir, "consensus/validator-bls.key")))
	assert.True(t, fileExists(path.Join(dir, "consensus/validator.sig")))
	assert.False(t, fileExists(path.Join(dir, "libp2p/libp2p.key")))

	ip.generatesNetwork = true
	res, err = ip.initKeys(sm)
	require.NoError(t, err)
	assert.Len(t, res, 1)

	assert.True(t, fileExists(path.Join(dir, "libp2p/libp2p.key")))
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}

	return !info.IsDir()
}
