//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package sharing

import (
	"fmt"
	"io"

	"github.com/coinbase/kryptology/pkg/core/curves"
)

type FeldmanVerifier struct {
	Commitments []curves.Point
}

func (v FeldmanVerifier) Verify(share *ShamirShare) error {
	curve := curves.GetCurveByName(v.Commitments[0].CurveName())
	err := share.Validate(curve)
	if err != nil {
		return err
	}
	x := curve.Scalar.New(int(share.Id))
	i := curve.Scalar.One()
	rhs := v.Commitments[0]

	for j := 1; j < len(v.Commitments); j++ {
		i = i.Mul(x)
		rhs = rhs.Add(v.Commitments[j].Mul(i))
	}
	sc, _ := curve.Scalar.SetBytes(share.Value)
	lhs := v.Commitments[0].Generator().Mul(sc)

	if lhs.Equal(rhs) {
		return nil
	} else {
		return fmt.Errorf("not equal")
	}
}

type Feldman struct {
	Threshold, Limit uint32
	Curve            *curves.Curve
}

func NewFeldman(threshold, limit uint32, curve *curves.Curve) (*Feldman, error) {
	if limit < threshold {
		return nil, fmt.Errorf("limit cannot be less than threshold")
	}
	if threshold < 2 {
		return nil, fmt.Errorf("threshold cannot be less than 2")
	}
	if limit > 255 {
		return nil, fmt.Errorf("cannot exceed 255 shares")
	}
	if curve == nil {
		return nil, fmt.Errorf("invalid curve")
	}
	return &Feldman{threshold, limit, curve}, nil
}

func (f Feldman) Split(secret curves.Scalar, reader io.Reader) (*FeldmanVerifier, []*ShamirShare, error) {
	if secret.IsZero() {
		return nil, nil, fmt.Errorf("invalid secret")
	}
	shamir := &Shamir{
		threshold: f.Threshold,
		limit:     f.Limit,
		curve:     f.Curve,
	}
	shares, poly := shamir.getPolyAndShares(secret, reader)
	verifier := new(FeldmanVerifier)
	verifier.Commitments = make([]curves.Point, f.Threshold)
	for i := range verifier.Commitments {
		verifier.Commitments[i] = f.Curve.ScalarBaseMult(poly.Coefficients[i])
	}
	return verifier, shares, nil
}

func (f Feldman) LagrangeCoeffs(shares map[uint32]*ShamirShare) (map[uint32]curves.Scalar, error) {
	shamir := &Shamir{
		threshold: f.Threshold,
		limit:     f.Limit,
		curve:     f.Curve,
	}
	identities := make([]uint32, 0)
	for _, xi := range shares {
		identities = append(identities, xi.Id)
	}
	return shamir.LagrangeCoeffs(identities)
}

func (f Feldman) Combine(shares ...*ShamirShare) (curves.Scalar, error) {
	shamir := &Shamir{
		threshold: f.Threshold,
		limit:     f.Limit,
		curve:     f.Curve,
	}
	return shamir.Combine(shares...)
}

func (f Feldman) CombinePoints(shares ...*ShamirShare) (curves.Point, error) {
	shamir := &Shamir{
		threshold: f.Threshold,
		limit:     f.Limit,
		curve:     f.Curve,
	}
	return shamir.CombinePoints(shares...)
}
