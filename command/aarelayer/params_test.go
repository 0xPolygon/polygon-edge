package aarelayer

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_validateFlags_ErrorValidateIPPort(t *testing.T) {
	t.Parallel()

	p := aarelayerParams{}

	assert.ErrorContains(t, p.validateFlags(), "invalid address:")
	p.addr = "%^%:78"
	assert.ErrorContains(t, p.validateFlags(), "invalid address:")
	p.addr = "127.0.0.1:H"
	assert.ErrorContains(t, p.validateFlags(), "invalid address:")
}

func Test_validateFlags_SecretsError(t *testing.T) {
	t.Parallel()

	p := aarelayerParams{
		addr: "127.0.0.1:8289",
	}

	assert.ErrorContains(t, p.validateFlags(), "no config file or data directory passed in")
}

func Test_validateFlags_ErrorValidateDbPath(t *testing.T) {
	t.Parallel()

	p := aarelayerParams{
		dbPath: "/tmp/non_existing_path_to/non_existing_file.db",
		addr:   "127.0.0.1:8289",
	}

	assert.ErrorContains(t, p.validateFlags(), "no such file or directory")

	p.dbPath = ""
	assert.ErrorContains(t, p.validateFlags(), "file name for boltdb not specified")
}

func Test_validateFlags_HappyPath(t *testing.T) {
	t.Parallel()

	tmpFilePath, err := os.MkdirTemp("/tmp", "aa_test_test_happy_path")
	require.NoError(t, err)

	defer os.RemoveAll(tmpFilePath)

	p := aarelayerParams{
		addr:       "127.0.0.1:8289",
		dbPath:     path.Join(tmpFilePath, "e.db"),
		configPath: "something",
	}

	assert.NoError(t, p.validateFlags())

	p.dbPath = "dummy.db"
	assert.NoError(t, p.validateFlags())
}
