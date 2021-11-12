// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package keystore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/ChainSafe/chainbridge-utils/crypto"
	"github.com/ChainSafe/chainbridge-utils/crypto/secp256k1"
	sr25519 "github.com/ChainSafe/chainbridge-utils/crypto/sr25519"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/ssh/terminal"
)

type EncryptedKeystore struct {
	Type       string `json:"type"`
	PublicKey  string `json:"publicKey"`
	Address    string `json:"address"`
	Ciphertext []byte `json:"ciphertext"`
}

// gcmFromPassphrase creates a symmetric AES key given a password
func gcmFromPassphrase(password []byte) (cipher.AEAD, error) {
	hash := blake2b.Sum256(password)

	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return gcm, nil
}

// Encrypt uses AES to encrypt `msg` with the symmetric key deterministically created from `password`
func Encrypt(msg, password []byte) ([]byte, error) {
	gcm, err := gcmFromPassphrase(password)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, msg, nil)
	return ciphertext, nil
}

// EncryptKeypair uses AES to encrypt an encoded `crypto.Keypair` with a symmetric key deterministically
// created from `password`
func EncryptKeypair(kp crypto.Keypair, password []byte) ([]byte, error) {
	return Encrypt(kp.Encode(), password)
}

// EncryptAndWriteToFile encrypts the `crypto.PrivateKey` using the password and saves it to the specified file
func EncryptAndWriteToFile(file *os.File, kp crypto.Keypair, password []byte) error {
	ciphertext, err := EncryptKeypair(kp, password)
	if err != nil {
		return err
	}

	keytype := ""

	if _, ok := kp.(*sr25519.Keypair); ok {
		keytype = crypto.Sr25519Type
	}

	if _, ok := kp.(*secp256k1.Keypair); ok {
		keytype = crypto.Secp256k1Type
	}

	if keytype == "" {
		return errors.New("cannot write key not of type secp256k1 or sr25519")
	}

	keydata := &EncryptedKeystore{
		Type:       keytype,
		PublicKey:  kp.PublicKey(),
		Address:    kp.Address(),
		Ciphertext: ciphertext,
	}

	data, err := json.MarshalIndent(keydata, "", "\t")
	if err != nil {
		return err
	}

	_, err = file.Write(append(data, byte('\n')))
	return err
}

// prompt user to enter password for encrypted keystore
func GetPassword(msg string) []byte {
	for {
		fmt.Println(msg)
		fmt.Print("> ")
		password, err := terminal.ReadPassword(int(syscall.Stdin))
		if err != nil {
			fmt.Printf("invalid input: %s\n", err)
		} else {
			fmt.Printf("\n")
			return password
		}
	}
}
