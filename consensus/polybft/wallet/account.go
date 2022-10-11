package wallet

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
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

func GenerateNewAccountFromSecret(secretManager secrets.SecretsManager, key string) (*Account, error) {
	// read account
	accountBytes, err := secretManager.GetSecret(key)
	if err != nil {
		return nil, fmt.Errorf("failed to read account data: %w", err)
	}

	return NewAccountFromBytes(accountBytes)
}

// NewAccountFromBytes creates a new Account from bytes
func NewAccountFromBytes(content []byte) (*Account, error) {
	var stored *keystoreAccount
	if err := json.Unmarshal(content, &stored); err != nil {
		return nil, err
	}

	ecdsaRaw, err := hex.DecodeString(stored.EcdsaPrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ecdsa: %w", err)
	}

	blsRaw, err := hex.DecodeString(stored.BlsPrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode bls: %w", err)
	}

	ecdsaKey, err := wallet.NewWalletFromPrivKey(ecdsaRaw)
	if err != nil {
		return nil, err
	}

	blsKey, err := bls.UnmarshalPrivateKey(blsRaw)
	if err != nil {
		return nil, err
	}

	return &Account{Ecdsa: ecdsaKey, Bls: blsKey}, nil
}

type keystoreAccount struct {
	EcdsaPrivKey string `json:"ecdsa"`
	BlsPrivKey   string `json:"bls"`
}

// SaveAccount saves an account to the given path
func (a *Account) ToBytes() ([]byte, error) {
	// get serialized ecdsa private key
	ecdsaRaw, err := a.Ecdsa.MarshallPrivateKey()
	if err != nil {
		return nil, err
	}

	// get serialized bls private key
	blsRaw, err := a.Bls.MarshalJSON()
	if err != nil {
		return nil, err
	}

	stored := &keystoreAccount{
		EcdsaPrivKey: hex.EncodeToString(ecdsaRaw),
		BlsPrivKey:   hex.EncodeToString(blsRaw),
	}

	return json.Marshal(stored)
}

func (a *Account) GetEcdsaPrivateKey() (*ecdsa.PrivateKey, error) {
	ecdsaRaw, err := a.Ecdsa.MarshallPrivateKey()
	if err != nil {
		return nil, err
	}

	return wallet.ParsePrivateKey(ecdsaRaw)
}
