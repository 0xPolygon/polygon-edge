//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

// Package sharing is an implementation of shamir secret sharing and implements the following papers.
//
// - https://dl.acm.org/doi/pdf/10.1145/359168.359176
// - https://www.cs.umd.edu/~gasarch/TOPICS/secretsharing/feldmanVSS.pdf
// - https://link.springer.com/content/pdf/10.1007%2F3-540-46766-1_9.pdf
package sharing

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/coinbase/kryptology/pkg/core/curves"
)

type ShamirShare struct {
	Id    uint32 `json:"identifier"`
	Value []byte `json:"value"`
}

func (ss ShamirShare) Validate(curve *curves.Curve) error {
	if ss.Id == 0 {
		return fmt.Errorf("invalid identifier")
	}
	sc, err := curve.Scalar.SetBytes(ss.Value)
	if err != nil {
		return err
	}
	if sc.IsZero() {
		return fmt.Errorf("invalid share")
	}
	return nil
}

func (ss ShamirShare) Bytes() []byte {
	var id [4]byte
	binary.BigEndian.PutUint32(id[:], ss.Id)
	return append(id[:], ss.Value...)
}

type Shamir struct {
	threshold, limit uint32
	curve            *curves.Curve
}

func NewShamir(threshold, limit uint32, curve *curves.Curve) (*Shamir, error) {
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
	return &Shamir{threshold, limit, curve}, nil
}

func (s Shamir) Split(secret curves.Scalar, reader io.Reader) ([]*ShamirShare, error) {
	if secret.IsZero() {
		return nil, fmt.Errorf("invalid secret")
	}
	shares, _ := s.getPolyAndShares(secret, reader)
	return shares, nil
}

func (s Shamir) getPolyAndShares(secret curves.Scalar, reader io.Reader) ([]*ShamirShare, *Polynomial) {
	poly := new(Polynomial).Init(secret, s.threshold, reader)
	shares := make([]*ShamirShare, s.limit)
	for i := range shares {
		x := s.curve.Scalar.New(i + 1)
		shares[i] = &ShamirShare{
			Id:    uint32(i + 1),
			Value: poly.Evaluate(x).Bytes(),
		}
	}
	return shares, poly
}

func (s Shamir) LagrangeCoeffs(identities []uint32) (map[uint32]curves.Scalar, error) {
	xs := make(map[uint32]curves.Scalar, len(identities))
	for _, xi := range identities {
		xs[xi] = s.curve.Scalar.New(int(xi))
	}

	result := make(map[uint32]curves.Scalar, len(identities))
	for i, xi := range xs {
		num := s.curve.Scalar.One()
		den := s.curve.Scalar.One()
		for j, xj := range xs {
			if i == j {
				continue
			}

			num = num.Mul(xj)
			den = den.Mul(xj.Sub(xi))
		}
		if den.IsZero() {
			return nil, fmt.Errorf("divide by zero")
		}
		result[i] = num.Div(den)
	}
	return result, nil
}

func (s Shamir) Combine(shares ...*ShamirShare) (curves.Scalar, error) {
	if len(shares) < int(s.threshold) {
		return nil, fmt.Errorf("invalid number of shares")
	}
	dups := make(map[uint32]bool, len(shares))
	xs := make([]curves.Scalar, len(shares))
	ys := make([]curves.Scalar, len(shares))

	for i, share := range shares {
		err := share.Validate(s.curve)
		if err != nil {
			return nil, err
		}
		if share.Id > s.limit {
			return nil, fmt.Errorf("invalid share identifier")
		}
		if _, in := dups[share.Id]; in {
			return nil, fmt.Errorf("duplicate share")
		}
		dups[share.Id] = true
		ys[i], _ = s.curve.Scalar.SetBytes(share.Value)
		xs[i] = s.curve.Scalar.New(int(share.Id))
	}
	return s.interpolate(xs, ys)
}

func (s Shamir) CombinePoints(shares ...*ShamirShare) (curves.Point, error) {
	if len(shares) < int(s.threshold) {
		return nil, fmt.Errorf("invalid number of shares")
	}
	dups := make(map[uint32]bool, len(shares))
	xs := make([]curves.Scalar, len(shares))
	ys := make([]curves.Point, len(shares))

	for i, share := range shares {
		err := share.Validate(s.curve)
		if err != nil {
			return nil, err
		}
		if share.Id > s.limit {
			return nil, fmt.Errorf("invalid share identifier")
		}
		if _, in := dups[share.Id]; in {
			return nil, fmt.Errorf("duplicate share")
		}
		dups[share.Id] = true
		sc, _ := s.curve.Scalar.SetBytes(share.Value)
		ys[i] = s.curve.ScalarBaseMult(sc)
		xs[i] = s.curve.Scalar.New(int(share.Id))
	}
	return s.interpolatePoint(xs, ys)
}

func (s Shamir) interpolate(xs, ys []curves.Scalar) (curves.Scalar, error) {
	result := s.curve.Scalar.Zero()
	for i, xi := range xs {
		num := s.curve.Scalar.One()
		den := s.curve.Scalar.One()
		for j, xj := range xs {
			if i == j {
				continue
			}
			num = num.Mul(xj)
			den = den.Mul(xj.Sub(xi))
		}
		if den.IsZero() {
			return nil, fmt.Errorf("divide by zero")
		}
		result = result.Add(ys[i].Mul(num.Div(den)))
	}
	return result, nil
}

func (s Shamir) interpolatePoint(xs []curves.Scalar, ys []curves.Point) (curves.Point, error) {
	result := s.curve.NewIdentityPoint()
	for i, xi := range xs {
		num := s.curve.Scalar.One()
		den := s.curve.Scalar.One()
		for j, xj := range xs {
			if i == j {
				continue
			}
			num = num.Mul(xj)
			den = den.Mul(xj.Sub(xi))
		}
		if den.IsZero() {
			return nil, fmt.Errorf("divide by zero")
		}
		result = result.Add(ys[i].Mul(num.Div(den)))
	}
	return result, nil
}
