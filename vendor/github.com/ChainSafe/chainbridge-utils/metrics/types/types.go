// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package types

import (
	"math/big"
	"time"
)

// LatestBlock is used to track the health of a chain
type LatestBlock struct {
	Height      *big.Int
	LastUpdated time.Time
}
