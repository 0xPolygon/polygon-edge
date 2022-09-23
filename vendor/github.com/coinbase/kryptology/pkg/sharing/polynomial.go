//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package sharing

import (
	"io"

	"github.com/coinbase/kryptology/pkg/core/curves"
)

type Polynomial struct {
	Coefficients []curves.Scalar
}

func (p *Polynomial) Init(intercept curves.Scalar, degree uint32, reader io.Reader) *Polynomial {
	p.Coefficients = make([]curves.Scalar, degree)
	p.Coefficients[0] = intercept.Clone()
	for i := 1; i < int(degree); i++ {
		p.Coefficients[i] = intercept.Random(reader)
	}
	return p
}

func (p Polynomial) Evaluate(x curves.Scalar) curves.Scalar {
	degree := len(p.Coefficients) - 1
	out := p.Coefficients[degree].Clone()
	for i := degree - 1; i >= 0; i-- {
		out = out.Mul(x).Add(p.Coefficients[i])
	}
	return out
}
