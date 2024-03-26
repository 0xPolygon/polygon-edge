package secrets

import "fmt"

type SecretsManagerMock struct {
	cache map[string][]byte
}

func NewSecretsManagerMock() *SecretsManagerMock {
	return &SecretsManagerMock{cache: make(map[string][]byte)}
}

func (sm *SecretsManagerMock) Setup() error {
	return nil
}

func (sm *SecretsManagerMock) GetSecret(name string) ([]byte, error) {
	value, exists := sm.cache[name]
	if !exists {
		return nil, fmt.Errorf("secret does not exists for %s", name)
	}

	return value, nil
}

func (sm *SecretsManagerMock) SetSecret(name string, value []byte) error {
	sm.cache[name] = value

	return nil
}

func (sm *SecretsManagerMock) HasSecret(name string) bool {
	_, exists := sm.cache[name]

	return exists
}

func (sm *SecretsManagerMock) RemoveSecret(name string) error {
	delete(sm.cache, name)

	return nil
}
