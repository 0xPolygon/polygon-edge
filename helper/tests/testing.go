package tests

import (
	"crypto/ecdsa"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
	"testing"
)

func GenerateKeyAndAddr(t *testing.T) (*ecdsa.PrivateKey, types.Address) {
	t.Helper()
	key, err := crypto.GenerateKey()
	assert.NoError(t, err)
	addr := crypto.PubKeyToAddress(&key.PublicKey)
	return key, addr
}
