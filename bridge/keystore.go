package bridge

import (
	"crypto/ecdsa"
	"fmt"
	"os"
	"path"

	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/ChainSafe/chainbridge-utils/crypto/secp256k1"
	"github.com/ChainSafe/chainbridge-utils/keystore"
)

func SaveKey(dirPath string, key *ecdsa.PrivateKey) error {
	address := crypto.PubKeyToAddress(&key.PublicKey)
	keyPath := path.Join(dirPath, fmt.Sprintf("%s.key", address.String()))

	fp, err := os.Create(keyPath)
	if err != nil {
		return err
	}
	kp := secp256k1.NewKeypair(*key)
	keystore.EncryptAndWriteToFile(fp, kp, []byte("password"))
	return nil
}
