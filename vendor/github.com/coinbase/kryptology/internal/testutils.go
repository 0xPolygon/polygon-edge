//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package internal

import (
	"math/big"
)

// B10 creating a big.Int from a base 10 string. panics on failure to
// ensure zero-values aren't used in place of malformed strings.
func B10(s string) *big.Int {
	x, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("Couldn't derive big.Int from string")
	}
	return x
}
