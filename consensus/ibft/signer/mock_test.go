package signer

import "github.com/0xPolygon/polygon-edge/secrets"

type MockSecretManager struct {
	// skip implementing the methods not to be used
	secrets.SecretsManager

	HasSecretFn func(string) bool
	GetSecretFn func(string) ([]byte, error)
	SetSecretFn func(string, []byte) error
}

func (m *MockSecretManager) HasSecret(name string) bool {
	return m.HasSecretFn(name)
}

func (m *MockSecretManager) GetSecret(name string) ([]byte, error) {
	return m.GetSecretFn(name)
}

func (m *MockSecretManager) SetSecret(name string, key []byte) error {
	return m.SetSecretFn(name, key)
}
