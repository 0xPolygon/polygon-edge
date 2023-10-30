package wallet

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/wallet"
)

// Account is an account for key signatures
type Account struct {
	Ecdsa *wallet.Key
	Bls   *bls.PrivateKey
}

// GenerateAccount generates a new random account
func GenerateAccount() (*Account, error) {
	key, err := wallet.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("cannot generate key. error: %w", err)
	}

	blsKey, err := bls.GenerateBlsKey()
	if err != nil {
		return nil, fmt.Errorf("cannot generate bls key. error: %w", err)
	}

	return &Account{
		Ecdsa: key,
		Bls:   blsKey,
	}, nil
}

// NewAccountFromSecret creates new account by using provided secretsManager
func NewAccountFromSecret(secretsManager secrets.SecretsManager) (*Account, error) {
	ecdsaKey, err := GetEcdsaFromSecret(secretsManager)
	if err != nil {
		return nil, err
	}

	blsKey, err := GetBlsFromSecret(secretsManager)
	if err != nil {
		return nil, err
	}

	return &Account{Ecdsa: ecdsaKey, Bls: blsKey}, nil
}

// GetEcdsaFromSecret retrieves validator(ECDSA) key by using provided secretsManager
func GetEcdsaFromSecret(secretsManager secrets.SecretsManager) (*wallet.Key, error) {
	encodedKey, err := secretsManager.GetSecret(secrets.ValidatorKey)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve ecdsa key: %w", err)
	}

	ecdsaRaw, err := hex.DecodeString(string(encodedKey))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve ecdsa key: %w", err)
	}

	key, err := wallet.NewWalletFromPrivKey(ecdsaRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve ecdsa key: %w", err)
	}

	return key, nil
}

// GetBlsFromSecret retrieves BLS key by using provided secretsManager
func GetBlsFromSecret(secretsManager secrets.SecretsManager) (*bls.PrivateKey, error) {
	encodedKey, err := secretsManager.GetSecret(secrets.ValidatorBLSKey)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve bls key: %w", err)
	}

	blsKey, err := bls.UnmarshalPrivateKey(encodedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve bls key: %w", err)
	}

	return blsKey, nil
}

// Save persists ECDSA and BLS private keys to the SecretsManager
func (a *Account) Save(secretsManager secrets.SecretsManager) (err error) {
	var (
		ecdsaRaw []byte
		blsRaw   []byte
	)

	// get serialized ecdsa private key
	if ecdsaRaw, err = a.Ecdsa.MarshallPrivateKey(); err != nil {
		return err
	}

	if err = secretsManager.SetSecret(secrets.ValidatorKey, []byte(hex.EncodeToString(ecdsaRaw))); err != nil {
		return err
	}

	// get serialized bls private key
	if blsRaw, err = a.Bls.Marshal(); err != nil {
		return err
	}

	return secretsManager.SetSecret(secrets.ValidatorBLSKey, blsRaw)
}

func (a *Account) GetEcdsaPrivateKey() (*ecdsa.PrivateKey, error) {
	ecdsaRaw, err := a.Ecdsa.MarshallPrivateKey()
	if err != nil {
		return nil, err
	}

	return wallet.ParsePrivateKey(ecdsaRaw)
}

func (a Account) Address() types.Address {
	return types.Address(a.Ecdsa.Address())
}
