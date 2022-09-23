//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
)

// GenerateSafePrime creates a prime number `p`
// where (`p`-1)/2 is also prime with at least `bits`
func GenerateSafePrime(bits uint) (*big.Int, error) {
	if bits < 3 {
		return nil, fmt.Errorf("safe prime size must be at least 3-bits")
	}

	var p *big.Int
	var err error
	checks := int(math.Max(float64(bits)/16, 8))
	for {
		// rand.Prime throws an error if bits < 2
		// -1 so the Sophie-Germain prime is 1023 bits
		// and the Safe prime is 1024
		p, err = rand.Prime(rand.Reader, int(bits)-1)
		if err != nil {
			return nil, err
		}
		p.Add(p.Lsh(p, 1), One)

		if p.ProbablyPrime(checks) {
			break
		}
	}

	return p, nil
}
