package wallet

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccount(t *testing.T) {
	key := GenerateAccount()
	bytes, err := key.ToBytes()
	require.NoError(t, err)

	pubKeyMarshalled := key.Bls.PublicKey().Marshal()
	privKeyMarshalled, err := key.Bls.MarshalJSON()
	require.NoError(t, err)

	key1, err := newAccountFromBytes(bytes)
	require.NoError(t, err)

	pubKeyMarshalled1 := key1.Bls.PublicKey().Marshal()
	privKeyMarshalled1, err := key1.Bls.MarshalJSON()
	require.NoError(t, err)

	assert.Equal(t, key.Ecdsa.Address(), key1.Ecdsa.Address())
	assert.Equal(t, pubKeyMarshalled, pubKeyMarshalled1)
	assert.Equal(t, privKeyMarshalled, privKeyMarshalled1)
}
