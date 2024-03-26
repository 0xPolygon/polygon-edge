package wallet

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccount(t *testing.T) {
	t.Parallel()

	secretsManager := secrets.NewSecretsManagerMock()

	key := generateTestAccount(t)
	pubKeyMarshalled := key.Bls.PublicKey().Marshal()
	privKeyMarshalled, err := key.Bls.Marshal()
	require.NoError(t, err)

	require.NoError(t, key.Save(secretsManager))

	key1, err := NewAccountFromSecret(secretsManager)
	require.NoError(t, err)

	pubKeyMarshalled1 := key1.Bls.PublicKey().Marshal()
	privKeyMarshalled1, err := key1.Bls.Marshal()
	require.NoError(t, err)

	assert.Equal(t, key.Ecdsa.Address(), key1.Ecdsa.Address())
	assert.Equal(t, pubKeyMarshalled, pubKeyMarshalled1)
	assert.Equal(t, privKeyMarshalled, privKeyMarshalled1)
}

func generateTestAccount(t *testing.T) *Account {
	t.Helper()

	acc, err := GenerateAccount()
	require.NoError(t, err)

	return acc
}
