// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package keystore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/ChainSafe/chainbridge-utils/crypto"
	"github.com/ChainSafe/chainbridge-utils/crypto/secp256k1"
	"github.com/ChainSafe/chainbridge-utils/crypto/sr25519"
)

// Decrypt uses AES to decrypt ciphertext with the symmetric key deterministically created from `password`
func Decrypt(data, password []byte) ([]byte, error) {
	gcm, err := gcmFromPassphrase(password)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		if err.Error() == "cipher: message authentication failed" {
			err = errors.New(err.Error() + ". Incorrect Password.")
		}
		return nil, err
	}

	return plaintext, nil
}

// DecodeKeypair turns input bytes into a keypair based on the specified key type
func DecodeKeypair(in []byte, keytype crypto.KeyType) (kp crypto.Keypair, err error) {
	if keytype == crypto.Secp256k1Type {
		kp = &secp256k1.Keypair{}
		err = kp.Decode(in)
	} else if keytype == crypto.Sr25519Type {
		kp = &sr25519.Keypair{}
		err = kp.Decode(in)
	} else {
		return nil, errors.New("cannot decode key: invalid key type")
	}

	return kp, err
}

// DecryptPrivateKey uses AES to decrypt the ciphertext into a `crypto.PrivateKey` with a symmetric key deterministically
// created from `password`
func DecryptKeypair(expectedPubK string, data, password []byte, keytype string) (crypto.Keypair, error) {
	pk, err := Decrypt(data, password)
	if err != nil {
		return nil, err
	}
	kp, err := DecodeKeypair(pk, keytype)
	if err != nil {
		return nil, err
	}

	// Check that the decoding matches what was expected
	if kp.PublicKey() != expectedPubK {
		return nil, fmt.Errorf("unexpected key file data, file may be corrupt or have been tampered with")
	}
	return kp, nil
}

// ReadFromFileAndDecrypt reads ciphertext from a file and decrypts it using the password into a `crypto.PrivateKey`
func ReadFromFileAndDecrypt(filename string, password []byte, keytype string) (crypto.Keypair, error) {
	fp, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(filepath.Clean(fp))
	if err != nil {
		return nil, err
	}

	keydata := new(EncryptedKeystore)
	err = json.Unmarshal(data, keydata)
	if err != nil {
		return nil, err
	}

	if keytype != keydata.Type {
		return nil, fmt.Errorf("Keystore type and Chain type mismatched. Expected Keystore file of type %s, got type %s", keytype, keydata.Type)
	}

	return DecryptKeypair(keydata.PublicKey, keydata.Ciphertext, password, keydata.Type)
}
