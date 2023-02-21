package wallet

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"

	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/umbracle/ethgo/wallet"
)

// Account is an account for key signatures
type Account struct {
	Ecdsa *wallet.Key
	Bls   *bls.PrivateKey
}

// GenerateAccount generates a new random account
func GenerateAccount() *Account {
	key, err := wallet.GenerateKey()
	if err != nil {
		panic("Cannot generate key")
	}

	blsKey, err := bls.GenerateBlsKey()
	if err != nil {
		panic("Cannot generate bls key")
	}

	return &Account{
		Ecdsa: key,
		Bls:   blsKey,
	}
}

// NewAccountFromSecret creates new account by using provided secretsManager
func NewAccountFromSecret(secretsManager secrets.SecretsManager) (*Account, error) {
	var (
		encodedKey []byte
		err        error
	)

	// ECDSA
	if encodedKey, err = secretsManager.GetSecret(secrets.ValidatorKey); err != nil {
		return nil, fmt.Errorf("failed to read account data: %w", err)
	}

	ecdsaRaw, err := hex.DecodeString(string(encodedKey))
	if err != nil {
		return nil, err
	}

	ecdsaKey, err := wallet.NewWalletFromPrivKey(ecdsaRaw)
	if err != nil {
		return nil, err
	}

	// BLS
	if encodedKey, err = secretsManager.GetSecret(secrets.ValidatorBLSKey); err != nil {
		return nil, fmt.Errorf("failed to read account data: %w", err)
	}

	blsKey, err := bls.UnmarshalPrivateKey(encodedKey)
	if err != nil {
		return nil, err
	}

	return &Account{Ecdsa: ecdsaKey, Bls: blsKey}, nil
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
