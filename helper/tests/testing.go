package tests

import (
	"crypto/ecdsa"
	"testing"

	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/stretchr/testify/assert"
)

func GenerateKeyAndAddr(t *testing.T) (*ecdsa.PrivateKey, types.Address) {
	t.Helper()
	key, err := crypto.GenerateKey()
	assert.NoError(t, err)
	addr := crypto.PubKeyToAddress(&key.PublicKey)
	return key, addr
}
