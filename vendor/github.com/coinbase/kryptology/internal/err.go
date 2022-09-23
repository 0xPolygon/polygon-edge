//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package internal

import "fmt"

var (
	ErrNotOnCurve           = fmt.Errorf("point not on the curve")
	ErrPointsDistinctCurves = fmt.Errorf("points must be from the same curve")
	ErrZmMembership         = fmt.Errorf("x ∉ Z_m")
	ErrResidueOne           = fmt.Errorf("value must be 1 (mod N)")
	ErrNCannotBeZero        = fmt.Errorf("N cannot be 0")
	ErrNilArguments         = fmt.Errorf("arguments cannot be nil")
	ErrZeroValue            = fmt.Errorf("arguments cannot be 0")
	ErrInvalidRound         = fmt.Errorf("invalid round method called")
	ErrIncorrectCount       = fmt.Errorf("incorrect number of inputs")
	ErrInvalidJson          = fmt.Errorf("json format does not contain the necessary data")
)
