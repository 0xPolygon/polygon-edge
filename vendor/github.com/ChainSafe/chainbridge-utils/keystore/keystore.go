// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

/*
The keystore package is used to load keys from keystore files, both for live use and for testing.

The Keystore

The keystore file is used as a file representation of a key. It contains 4 parts:
- The key type (secp256k1, sr25519)
- The PublicKey
- The Address
- The ciphertext

This keystore also requires a password to decrypt into a usable key.
The keystore library can be used to both encrypt keys into keystores, and decrypt keystore into keys.
For more information on how to encrypt and decrypt from the command line, reference the README: https://github.com/ChainSafe/ChainBridge

The Keyring

The keyring provides predefined secp256k1 and srr25519 keys to use in testing.
These keys are automatically provided during runtime and stored in memory rather than being stored on disk.
There are 5 keys currenty supported: Alice, Bob, Charlie, Dave, and Eve.

*/
package keystore

import (
	"fmt"
	"os"

	"github.com/ChainSafe/chainbridge-utils/crypto"
)

const EnvPassword = "KEYSTORE_PASSWORD"

var keyMapping = map[string]string{
	"ethereum":  "secp256k1",
	"substrate": "sr25519",
}

// KeypairFromAddress attempts to load the encrypted key file for the provided address,
// prompting the user for the password.
func KeypairFromAddress(addr, chainType, path string, insecure bool) (crypto.Keypair, error) {
	if insecure {
		return insecureKeypairFromAddress(path, chainType)
	}
	path = fmt.Sprintf("%s/%s.key", path, addr)
	// Make sure key exists before prompting password
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("key file not found: %s", path)
	}

	var pswd []byte
	if pswdStr := os.Getenv(EnvPassword); pswdStr != "" {
		pswd = []byte(pswdStr)
	} else {
		pswd = GetPassword(fmt.Sprintf("Enter password for key %s:", path))
	}

	kp, err := ReadFromFileAndDecrypt(path, pswd, keyMapping[chainType])
	if err != nil {
		return nil, err
	}

	return kp, nil
}
