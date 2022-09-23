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

// Pedersen Verifiable Secret Sharing Scheme
type Pedersen struct {
	threshold, limit uint32
	curve            *curves.Curve
	generator        curves.Point
}

type PedersenVerifier struct {
	Generator   curves.Point
	Commitments []curves.Point
}

func (pv PedersenVerifier) Verify(share, blindShare *ShamirShare) error {
	curve := curves.GetCurveByName(pv.Generator.CurveName())
	if err := share.Validate(curve); err != nil {
		return err
	}
	if err := blindShare.Validate(curve); err != nil {
		return err
	}

	x := curve.Scalar.New(int(share.Id))
	i := curve.Scalar.One()
	rhs := pv.Commitments[0]

	for j := 1; j < len(pv.Commitments); j++ {
		i = i.Mul(x)
		rhs = rhs.Add(pv.Commitments[j].Mul(i))
	}

	sc, _ := curve.Scalar.SetBytes(share.Value)
	bsc, _ := curve.Scalar.SetBytes(blindShare.Value)
	g := pv.Commitments[0].Generator().Mul(sc)
	h := pv.Generator.Mul(bsc)
	lhs := g.Add(h)

	if lhs.Equal(rhs) {
		return nil
	} else {
		return fmt.Errorf("not equal")
	}
}

// PedersenResult contains all the data from calling Split
type PedersenResult struct {
	Blinding                     curves.Scalar
	BlindingShares, SecretShares []*ShamirShare
	FeldmanVerifier              *FeldmanVerifier
	PedersenVerifier             *PedersenVerifier
}

// NewPedersen creates a new pedersen VSS
func NewPedersen(threshold, limit uint32, generator curves.Point) (*Pedersen, error) {
	if limit < threshold {
		return nil, fmt.Errorf("limit cannot be less than threshold")
	}
	if threshold < 2 {
		return nil, fmt.Errorf("threshold cannot be less than 2")
	}
	if limit > 255 {
		return nil, fmt.Errorf("cannot exceed 255 shares")
	}
	curve := curves.GetCurveByName(generator.CurveName())
	if curve == nil {
		return nil, fmt.Errorf("invalid curve")
	}
	if generator == nil {
		return nil, fmt.Errorf("invalid generator")
	}
	if !generator.IsOnCurve() || generator.IsIdentity() {
		return nil, fmt.Errorf("invalid generator")
	}
	return &Pedersen{threshold, limit, curve, generator}, nil
}

// Split creates the verifiers, blinding and shares
func (pd Pedersen) Split(secret curves.Scalar, reader io.Reader) (*PedersenResult, error) {
	// generate a random blinding factor
	blinding := pd.curve.Scalar.Random(reader)

	shamir := Shamir{pd.threshold, pd.limit, pd.curve}
	// split the secret into shares
	shares, poly := shamir.getPolyAndShares(secret, reader)

	// split the blinding into shares
	blindingShares, polyBlinding := shamir.getPolyAndShares(blinding, reader)

	// Generate the verifiable commitments to the polynomial for the shares
	blindedverifiers := make([]curves.Point, pd.threshold)
	verifiers := make([]curves.Point, pd.threshold)

	// ({p0 * G + b0 * H}, ...,{pt * G + bt * H})
	for i, c := range poly.Coefficients {
		s := pd.curve.ScalarBaseMult(c)
		b := pd.generator.Mul(polyBlinding.Coefficients[i])
		bv := s.Add(b)
		blindedverifiers[i] = bv
		verifiers[i] = s
	}
	verifier1 := &FeldmanVerifier{Commitments: verifiers}
	verifier2 := &PedersenVerifier{Commitments: blindedverifiers, Generator: pd.generator}

	return &PedersenResult{
		blinding, blindingShares, shares, verifier1, verifier2,
	}, nil
}

func (pd Pedersen) LagrangeCoeffs(shares map[uint32]*ShamirShare) (map[uint32]curves.Scalar, error) {
	shamir := &Shamir{
		threshold: pd.threshold,
		limit:     pd.limit,
		curve:     pd.curve,
	}
	identities := make([]uint32, 0)
	for _, xi := range shares {
		identities = append(identities, xi.Id)
	}
	return shamir.LagrangeCoeffs(identities)
}

func (pd Pedersen) Combine(shares ...*ShamirShare) (curves.Scalar, error) {
	shamir := &Shamir{
		threshold: pd.threshold,
		limit:     pd.limit,
		curve:     pd.curve,
	}
	return shamir.Combine(shares...)
}

func (pd Pedersen) CombinePoints(shares ...*ShamirShare) (curves.Point, error) {
	shamir := &Shamir{
		threshold: pd.threshold,
		limit:     pd.limit,
		curve:     pd.curve,
	}
	return shamir.CombinePoints(shares...)
}
