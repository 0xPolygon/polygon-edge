package wallet

import (
	"fmt"
	"testing"

	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccount(t *testing.T) {
	t.Parallel()

	secretsManager := newSecretsManagerMock()

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

func newSecretsManagerMock() secrets.SecretsManager {
	return &secretsManagerMock{cache: make(map[string][]byte)}
}

func generateTestAccount(t *testing.T) *Account {
	t.Helper()

	acc, err := GenerateAccount()
	require.NoError(t, err)

	return acc
}

type secretsManagerMock struct {
	cache map[string][]byte
}

func (sm *secretsManagerMock) Setup() error {
	return nil
}

func (sm *secretsManagerMock) GetSecret(name string) ([]byte, error) {
	value, exists := sm.cache[name]
	if !exists {
		return nil, fmt.Errorf("secret does not exists for %s", name)
	}

	return value, nil
}

func (sm *secretsManagerMock) SetSecret(name string, value []byte) error {
	sm.cache[name] = value

	return nil
}

func (sm *secretsManagerMock) HasSecret(name string) bool {
	_, exists := sm.cache[name]

	return exists
}

func (sm *secretsManagerMock) RemoveSecret(name string) error {
	delete(sm.cache, name)

	return nil
}
