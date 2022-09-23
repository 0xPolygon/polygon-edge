//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"crypto/hmac"
	crand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"hash"
)

// Size of random values and hash outputs are determined by our hash function
const Size = sha256.Size

type (
	// Commitment to a given message which can be later revealed.
	// This is sent to and held by a verifier until the corresponding
	// witness is provided.
	Commitment []byte

	// Witness is sent to and opened by the verifier. This proves that
	// committed message hasn't been altered by later information.
	Witness struct {
		Msg []byte
		r   [Size]byte
	}

	// witnessJSON is used for un/marshaling.
	witnessJSON struct {
		Msg []byte
		R   [Size]byte
	}
)

// MarshalJSON encodes Witness in JSON
func (w Witness) MarshalJSON() ([]byte, error) {
	return json.Marshal(witnessJSON{w.Msg, w.r})
}

// UnmarshalJSON decodes JSON into a Witness struct
func (w *Witness) UnmarshalJSON(data []byte) error {
	witness := &witnessJSON{}
	err := json.Unmarshal(data, witness)
	if err != nil {
		return err
	}
	w.Msg = witness.Msg
	w.r = witness.R
	return nil
}

// Commit to a given message. Uses SHA256 as the hash function.
func Commit(msg []byte) (Commitment, *Witness, error) {
	// Initialize our decommitment
	d := Witness{msg, [Size]byte{}}

	// Generate a random nonce of the required length
	n, err := crand.Read(d.r[:])
	// Ensure no errors retrieving nonce
	if err != nil {
		return nil, nil, err
	}

	// Ensure we read all the bytes expected
	if n != Size {
		return nil, nil, fmt.Errorf("failed to read %v bytes from crypto/rand: received %v bytes", Size, n)
	}
	// Compute the commitment: HMAC(Sha2, msg, key)
	c, err := ComputeHMAC(sha256.New, msg, d.r[:])
	if err != nil {
		return nil, nil, err
	}
	return c, &d, nil
}

// Open a commitment and return true if the commitment/decommitment pair are valid.
// reference: spec.§2.4: Commitment Scheme
func Open(c Commitment, d Witness) (bool, error) {
	// Ensure commitment is well-formed.
	if len(c) != Size {
		return false, fmt.Errorf("invalid commitment, wrong length. %v != %v", len(c), Size)
	}

	// Re-compute the commitment: HMAC(Sha2, msg, key)
	cʹ, err := ComputeHMAC(sha256.New, d.Msg, d.r[:])
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(cʹ, c) == 1, nil
}

// ComputeHMAC computes HMAC(hash_fn, msg, key)
// Takes in a hash function to use for HMAC
func ComputeHMAC(f func() hash.Hash, msg []byte, k []byte) ([]byte, error) {
	if f == nil {
		return nil, fmt.Errorf("hash function cannot be nil")
	}

	mac := hmac.New(f, k)
	w, err := mac.Write(msg)

	if w != len(msg) {
		return nil, fmt.Errorf("bytes written to hash doesn't match expected: %v != %v", w, len(msg))
	} else if err != nil {
		return nil, err
	}
	return mac.Sum(nil), nil
}
