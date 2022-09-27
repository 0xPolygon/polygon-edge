package signer

import (
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
)

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

type MockKeyManager struct {
	TypeFunc                   func() validators.ValidatorType
	AddressFunc                func() types.Address
	NewEmptyValidatorsFunc     func() validators.Validators
	NewEmptyCommittedSealsFunc func() Seals
	SignProposerSealFunc       func([]byte) ([]byte, error)
	SignCommittedSealFunc      func([]byte) ([]byte, error)
	VerifyCommittedSealFunc    func(validators.Validators, types.Address, []byte, []byte) error
	GenerateCommittedSealsFunc func(map[types.Address][]byte, validators.Validators) (Seals, error)
	VerifyCommittedSealsFunc   func(Seals, []byte, validators.Validators) (int, error)
	SignIBFTMessageFunc        func([]byte) ([]byte, error)
	EcrecoverFunc              func([]byte, []byte) (types.Address, error)
}

func (m *MockKeyManager) Type() validators.ValidatorType {
	return m.TypeFunc()
}

func (m *MockKeyManager) Address() types.Address {
	return m.AddressFunc()
}

func (m *MockKeyManager) NewEmptyValidators() validators.Validators {
	return m.NewEmptyValidatorsFunc()
}

func (m *MockKeyManager) NewEmptyCommittedSeals() Seals {
	return m.NewEmptyCommittedSealsFunc()
}
func (m *MockKeyManager) SignProposerSeal(hash []byte) ([]byte, error) {
	return m.SignProposerSealFunc(hash)
}

func (m *MockKeyManager) SignCommittedSeal(hash []byte) ([]byte, error) {
	return m.SignCommittedSealFunc(hash)
}

func (m *MockKeyManager) VerifyCommittedSeal(vals validators.Validators, signer types.Address, sig, hash []byte) error {
	return m.VerifyCommittedSealFunc(vals, signer, sig, hash)
}

func (m *MockKeyManager) GenerateCommittedSeals(
	sealsByValidator map[types.Address][]byte,
	vals validators.Validators,
) (Seals, error) {
	return m.GenerateCommittedSealsFunc(sealsByValidator, vals)
}

func (m *MockKeyManager) VerifyCommittedSeals(seals Seals, hash []byte, vals validators.Validators) (int, error) {
	return m.VerifyCommittedSealsFunc(seals, hash, vals)
}

func (m *MockKeyManager) SignIBFTMessage(msg []byte) ([]byte, error) {
	return m.SignIBFTMessageFunc(msg)
}

func (m *MockKeyManager) Ecrecover(sig []byte, msg []byte) (types.Address, error) {
	return m.EcrecoverFunc(sig, msg)
}
