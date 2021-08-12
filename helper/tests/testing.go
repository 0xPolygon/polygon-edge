package tests

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
)

func EthToWei(ethValue int64) *big.Int {
	return new(big.Int).Mul(
		big.NewInt(ethValue),
		new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
}

func GenerateKeyAndAddr(t *testing.T) (*ecdsa.PrivateKey, types.Address) {
	t.Helper()
	key, err := crypto.GenerateKey()
	assert.NoError(t, err)
	addr := crypto.PubKeyToAddress(&key.PublicKey)
	return key, addr
}
