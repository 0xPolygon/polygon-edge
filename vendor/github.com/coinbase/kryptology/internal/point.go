//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package internal

import (
	"crypto/elliptic"
	"math/big"

	"filippo.io/edwards25519"
)

func CalcFieldSize(curve elliptic.Curve) int {
	bits := curve.Params().BitSize
	return (bits + 7) / 8
}

func ReverseScalarBytes(inBytes []byte) []byte {
	outBytes := make([]byte, len(inBytes))

	for i, j := 0, len(inBytes)-1; j >= 0; i, j = i+1, j-1 {
		outBytes[i] = inBytes[j]
	}

	return outBytes
}

func BigInt2Ed25519Point(y *big.Int) (*edwards25519.Point, error) {
	b := y.Bytes()
	var arr [32]byte
	copy(arr[32-len(b):], b)
	return edwards25519.NewIdentityPoint().SetBytes(arr[:])
}

func BigInt2Ed25519Scalar(x *big.Int) (*edwards25519.Scalar, error) {
	// big.Int is big endian; ed25519 assumes little endian encoding
	kBytes := ReverseScalarBytes(x.Bytes())
	var arr [32]byte
	copy(arr[:], kBytes)
	return edwards25519.NewScalar().SetCanonicalBytes(arr[:])
}
