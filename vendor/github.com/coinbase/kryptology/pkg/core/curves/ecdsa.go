//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package curves

import (
	"crypto/ecdsa"
	"math/big"
)

// EcdsaVerify runs a curve- or algorithm-specific ECDSA verification function on input
// an ECDSA public (verification) key, a message digest, and an ECDSA signature.
// It must return true if all the parameters are sane and the ECDSA signature is valid,
// and false otherwise
type EcdsaVerify func(pubKey *EcPoint, hash []byte, signature *EcdsaSignature) bool

// EcdsaSignature represents a (composite) digital signature
type EcdsaSignature struct {
	V    int
	R, S *big.Int
}

// Static type assertion
var _ EcdsaVerify = VerifyEcdsa

// Verifies ECDSA signature using core types.
func VerifyEcdsa(pk *EcPoint, hash []byte, sig *EcdsaSignature) bool {
	return ecdsa.Verify(
		&ecdsa.PublicKey{
			Curve: pk.Curve,
			X:     pk.X,
			Y:     pk.Y,
		},
		hash, sig.R, sig.S)
}
