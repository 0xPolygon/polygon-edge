package wallet

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAccount(t *testing.T) {
	tmpDir, err := ioutil.TempDir("/tmp", "account-")
	assert.NoError(t, err)

	keyPath := filepath.Join(tmpDir, "some.json")
	password := "qwerty"

	key := GenerateAccount()
	assert.NoError(t, key.SaveAccount(keyPath, password))
	pubKeyMarshalled := key.Bls.PublicKey().Marshal()
	privKeyMarshalled, err := key.Bls.MarshalJSON()
	assert.NoError(t, err)

	key1, err := NewAccount(keyPath, password)
	assert.NoError(t, err)

	pubKeyMarshalled1 := key1.Bls.PublicKey().Marshal()
	privKeyMarshalled1, err := key1.Bls.MarshalJSON()
	assert.NoError(t, err)

	assert.Equal(t, key.Ecdsa.Address(), key1.Ecdsa.Address())
	assert.Equal(t, pubKeyMarshalled, pubKeyMarshalled1)
	assert.Equal(t, privKeyMarshalled, privKeyMarshalled1)
}
