// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package keystore

import (
	"fmt"

	"github.com/ChainSafe/chainbridge-utils/crypto"
	"github.com/ChainSafe/chainbridge-utils/crypto/secp256k1"
	"github.com/ChainSafe/chainbridge-utils/crypto/sr25519"
	"github.com/centrifuge/go-substrate-rpc-client/signature"
)

// The Constant "keys". These are the name that the keys are based on. This can be expanded, but
// any additions must be added to Keys and to insecureKeyFromAddress
const AliceKey = "alice"
const BobKey = "bob"
const CharlieKey = "charlie"
const DaveKey = "dave"
const EveKey = "eve"

var Keys = []string{AliceKey, BobKey, CharlieKey, DaveKey, EveKey}

// The Chain type Constants
const EthChain = "ethereum"
const SubChain = "substrate"

var TestKeyRing *TestKeyRingHolder

var AliceSr25519 = sr25519.NewKeypairFromKRP(signature.KeyringPair{
	URI:       "//Alice",
	Address:   "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY",
	PublicKey: []byte{0xd4, 0x35, 0x93, 0xc7, 0x15, 0xfd, 0xd3, 0x1c, 0x61, 0x14, 0x1a, 0xbd, 0x4, 0xa9, 0x9f, 0xd6, 0x82, 0x2c, 0x85, 0x58, 0x85, 0x4c, 0xcd, 0xe3, 0x9a, 0x56, 0x84, 0xe7, 0xa5, 0x6d, 0xa2, 0x7d},
})

var BobSr25519 = sr25519.NewKeypairFromKRP(signature.KeyringPair{
	URI:       "//Bob",
	Address:   "5FHneW46xGXgs5mUiveU4sbTyGBzmstUspZC92UhjJM694ty",
	PublicKey: []byte{0x8e, 0xaf, 0x4, 0x15, 0x16, 0x87, 0x73, 0x63, 0x26, 0xc9, 0xfe, 0xa1, 0x7e, 0x25, 0xfc, 0x52, 0x87, 0x61, 0x36, 0x93, 0xc9, 0x12, 0x90, 0x9c, 0xb2, 0x26, 0xaa, 0x47, 0x94, 0xf2, 0x6a, 0x48},
})

var CharlieSr25519 = sr25519.NewKeypairFromKRP(signature.KeyringPair{
	URI:       "//Charlie",
	Address:   "5FLSigC9HGRKVhB9FiEo4Y3koPsNmBmLJbpXg2mp1hXcS59Y",
	PublicKey: []byte{0x90, 0xb5, 0xab, 0x20, 0x5c, 0x69, 0x74, 0xc9, 0xea, 0x84, 0x1b, 0xe6, 0x88, 0x86, 0x46, 0x33, 0xdc, 0x9c, 0xa8, 0xa3, 0x57, 0x84, 0x3e, 0xea, 0xcf, 0x23, 0x14, 0x64, 0x99, 0x65, 0xfe, 0x22},
})

var DaveSr25519 = sr25519.NewKeypairFromKRP(signature.KeyringPair{
	URI:       "//Dave",
	Address:   "5DAAnrj7VHTznn2AWBemMuyBwZWs6FNFjdyVXUeYum3PTXFy",
	PublicKey: []byte{0x30, 0x67, 0x21, 0x21, 0x1d, 0x54, 0x4, 0xbd, 0x9d, 0xa8, 0x8e, 0x2, 0x4, 0x36, 0xa, 0x1a, 0x9a, 0xb8, 0xb8, 0x7c, 0x66, 0xc1, 0xbc, 0x2f, 0xcd, 0xd3, 0x7f, 0x3c, 0x22, 0x22, 0xcc, 0x20},
})

var EveSr25519 = sr25519.NewKeypairFromKRP(signature.KeyringPair{
	URI:       "//Eve",
	Address:   "5HGjWAeFDfFCWPsjFQdVV2Msvz2XtMktvgocEZcCj68kUMaw",
	PublicKey: []byte{0xe6, 0x59, 0xa7, 0xa1, 0x62, 0x8c, 0xdd, 0x93, 0xfe, 0xbc, 0x4, 0xa4, 0xe0, 0x64, 0x6e, 0xa2, 0xe, 0x9f, 0x5f, 0xc, 0xe0, 0x97, 0xd9, 0xa0, 0x52, 0x90, 0xd4, 0xa9, 0xe0, 0x54, 0xdf, 0x4e},
})

// TestKeyStore is a struct that holds a Keystore of all the test keys
type TestKeyRingHolder struct {
	EthereumKeys  map[string]*secp256k1.Keypair
	SubstrateKeys map[string]*sr25519.Keypair
}

// Init function to create a keyRing that can be accessed anywhere without having to recreate the data
func init() {
	TestKeyRing = &TestKeyRingHolder{
		EthereumKeys: makeEthRing(),
		SubstrateKeys: map[string]*sr25519.Keypair{
			AliceKey:   AliceSr25519,
			BobKey:     BobSr25519,
			CharlieKey: CharlieSr25519,
			DaveKey:    DaveSr25519,
			EveKey:     EveSr25519,
		},
	}

}

func makeEthRing() map[string]*secp256k1.Keypair {
	ring := map[string]*secp256k1.Keypair{}
	for _, key := range Keys {
		bz := padWithZeros([]byte(key), secp256k1.PrivateKeyLength)
		kp, err := secp256k1.NewKeypairFromPrivateKey(bz)
		if err != nil {
			panic(err)
		}
		ring[key] = kp
	}

	return ring
}

// padWithZeros adds on extra 0 bytes to make a byte array of a specified length
func padWithZeros(key []byte, targetLength int) []byte {
	res := make([]byte, targetLength-len(key))
	return append(res, key...)
}

// insecureKeypairFromAddress is used for resolving addresses to test keypairs.
func insecureKeypairFromAddress(key string, chainType string) (crypto.Keypair, error) {
	var kp crypto.Keypair
	var ok bool

	if chainType == EthChain {
		kp, ok = TestKeyRing.EthereumKeys[key]
	} else if chainType == SubChain {
		kp, ok = TestKeyRing.SubstrateKeys[key]
	} else {
		return nil, fmt.Errorf("unrecognized chain type: %s", chainType)
	}

	if !ok {
		return nil, fmt.Errorf("invalid test key selection: %s", key)
	}

	return kp, nil
}
