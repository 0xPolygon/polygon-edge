package wallet

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"

	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/umbracle/ethgo/keystore"
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

func GenerateAccountFromSecrets(secretsManager secrets.SecretsManager) (*Account, error) {
	validatorBytes, err := secretsManager.GetSecret(secrets.ValidatorKey)
	if err != nil {
		return nil, err
	}

	blsBytes, err := secretsManager.GetSecret(secrets.ValidatorBLSKey)
	if err != nil {
		return nil, err
	}

	ecdsaPrivateKey, err := crypto.BytesToECDSAPrivateKey(validatorBytes)
	if err != nil {
		return nil, err
	}

	blsPrivateKey, err := bls.UnmarshalPrivateKey(blsBytes)
	if err != nil {
		return nil, err
	}

	return &Account{wallet.NewKey(ecdsaPrivateKey), blsPrivateKey}, nil
}

// NewAccount creates a new Account from a file
func NewAccount(path string, password string) (*Account, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return newAccountFromBytes(data, password)
}

// NewAccountFromBytes creates a new Account from bytes
func newAccountFromBytes(content []byte, password string) (*Account, error) {
	dst, err := keystore.DecryptV3(content, password)
	if err != nil {
		return nil, err
	}

	var stored *keystoreAccount
	if err := json.Unmarshal(dst, &stored); err != nil {
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
func (a *Account) SaveAccount(path string, password string) error {
	// get serialized ecdsa private key
	ecdsaRaw, err := a.Ecdsa.MarshallPrivateKey()
	if err != nil {
		return err
	}

	// get serialized bls private key
	blsRaw, err := a.Bls.MarshalJSON()
	if err != nil {
		return err
	}

	stored := &keystoreAccount{
		EcdsaPrivKey: hex.EncodeToString(ecdsaRaw),
		BlsPrivKey:   hex.EncodeToString(blsRaw),
	}

	raw, err := json.Marshal(stored)
	if err != nil {
		return err
	}

	keystoreRaw, err := keystore.EncryptV3(raw, password)
	if err != nil {
		return err
	}

	const fileMode = 0600
	if err := ioutil.WriteFile(path, keystoreRaw, fileMode); err != nil {
		return err
	}

	return nil
}
