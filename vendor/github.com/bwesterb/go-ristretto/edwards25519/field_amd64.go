// +build amd64,!go1.13,!forcegeneric

package edwards25519

//go:noescape
func feMul(out, a, b *FieldElement)

//go:noescape
func feSquare(out, a *FieldElement)

// Sets fe to a * b.  Returns fe.
func (fe *FieldElement) Mul(a, b *FieldElement) *FieldElement {
	feMul(fe, a, b)
	return fe
}

// Sets fe to a^2.  Returns fe.
func (fe *FieldElement) Square(a *FieldElement) *FieldElement {
	feSquare(fe, a)
	return fe
}

// Sets fe to 2 * a^2.  Returns fe.
func (fe *FieldElement) DoubledSquare(a *FieldElement) *FieldElement {
	feSquare(fe, a)
	return fe.add(fe, fe)
}
