// +build go1.13,!forcegeneric

package edwards25519

import (
	"math/bits"
)

// Sets fe to a * b.  Returns fe.
func (fe *FieldElement) Mul(a, b *FieldElement) *FieldElement {
	a0 := a[0]
	a1 := a[1]
	a2 := a[2]
	a3 := a[3]
	a4 := a[4]
	b0 := b[0]
	b1 := b[1]
	b2 := b[2]
	b3 := b[3]
	b4 := b[4]
	b1_19 := b1 * 19
	b2_19 := b2 * 19
	b3_19 := b3 * 19
	b4_19 := b4 * 19

	var carry, h, l uint64

	// TODO make sure we only use bits.Add64 if it's constant-time
	c0h, c0l := bits.Mul64(b0, a0)
	h, l = bits.Mul64(b1_19, a4)
	c0l, carry = bits.Add64(c0l, l, 0)
	c0h, _ = bits.Add64(c0h, h, carry)
	h, l = bits.Mul64(b2_19, a3)
	c0l, carry = bits.Add64(c0l, l, 0)
	c0h, _ = bits.Add64(c0h, h, carry)
	h, l = bits.Mul64(b3_19, a2)
	c0l, carry = bits.Add64(c0l, l, 0)
	c0h, _ = bits.Add64(c0h, h, carry)
	h, l = bits.Mul64(b4_19, a1)
	c0l, carry = bits.Add64(c0l, l, 0)
	c0h, _ = bits.Add64(c0h, h, carry)

	c1h, c1l := bits.Mul64(b0, a1)
	h, l = bits.Mul64(b1, a0)
	c1l, carry = bits.Add64(c1l, l, 0)
	c1h, _ = bits.Add64(c1h, h, carry)
	h, l = bits.Mul64(b2_19, a4)
	c1l, carry = bits.Add64(c1l, l, 0)
	c1h, _ = bits.Add64(c1h, h, carry)
	h, l = bits.Mul64(b3_19, a3)
	c1l, carry = bits.Add64(c1l, l, 0)
	c1h, _ = bits.Add64(c1h, h, carry)
	h, l = bits.Mul64(b4_19, a2)
	c1l, carry = bits.Add64(c1l, l, 0)
	c1h, _ = bits.Add64(c1h, h, carry)

	c1l, carry = bits.Add64((c0l>>51)|(c0h<<13), c1l, 0)
	c1h, _ = bits.Add64(c1h, 0, carry)
	c0l &= 0x7ffffffffffff

	c2h, c2l := bits.Mul64(b0, a2)
	h, l = bits.Mul64(b1, a1)
	c2l, carry = bits.Add64(c2l, l, 0)
	c2h, _ = bits.Add64(c2h, h, carry)
	h, l = bits.Mul64(b2, a0)
	c2l, carry = bits.Add64(c2l, l, 0)
	c2h, _ = bits.Add64(c2h, h, carry)
	h, l = bits.Mul64(b3_19, a4)
	c2l, carry = bits.Add64(c2l, l, 0)
	c2h, _ = bits.Add64(c2h, h, carry)
	h, l = bits.Mul64(b4_19, a3)
	c2l, carry = bits.Add64(c2l, l, 0)
	c2h, _ = bits.Add64(c2h, h, carry)

	c2l, carry = bits.Add64((c1l>>51)|(c1h<<13), c2l, 0)
	c2h, _ = bits.Add64(c2h, 0, carry)
	c1l &= 0x7ffffffffffff

	c3h, c3l := bits.Mul64(b0, a3)
	h, l = bits.Mul64(b1, a2)
	c3l, carry = bits.Add64(c3l, l, 0)
	c3h, _ = bits.Add64(c3h, h, carry)
	h, l = bits.Mul64(b2, a1)
	c3l, carry = bits.Add64(c3l, l, 0)
	c3h, _ = bits.Add64(c3h, h, carry)
	h, l = bits.Mul64(b3, a0)
	c3l, carry = bits.Add64(c3l, l, 0)
	c3h, _ = bits.Add64(c3h, h, carry)
	h, l = bits.Mul64(b4_19, a4)
	c3l, carry = bits.Add64(c3l, l, 0)
	c3h, _ = bits.Add64(c3h, h, carry)

	c3l, carry = bits.Add64((c2l>>51)|(c2h<<13), c3l, 0)
	c3h, _ = bits.Add64(c3h, 0, carry)
	c2l &= 0x7ffffffffffff

	c4h, c4l := bits.Mul64(b0, a4)
	h, l = bits.Mul64(b1, a3)
	c4l, carry = bits.Add64(c4l, l, 0)
	c4h, _ = bits.Add64(c4h, h, carry)
	h, l = bits.Mul64(b2, a2)
	c4l, carry = bits.Add64(c4l, l, 0)
	c4h, _ = bits.Add64(c4h, h, carry)
	h, l = bits.Mul64(b3, a1)
	c4l, carry = bits.Add64(c4l, l, 0)
	c4h, _ = bits.Add64(c4h, h, carry)
	h, l = bits.Mul64(b4, a0)
	c4l, carry = bits.Add64(c4l, l, 0)
	c4h, _ = bits.Add64(c4h, h, carry)

	c4l, carry = bits.Add64((c3l>>51)|(c3h<<13), c4l, 0)
	c4h, _ = bits.Add64(c4h, 0, carry)
	c3l &= 0x7ffffffffffff

	carry = ((c4l >> 51) | (c4h << 13))
	c4l &= 0x7ffffffffffff
	c0l += carry * 19
	c1l += c0l >> 51
	c0l &= 0x7ffffffffffff

	fe[0] = c0l
	fe[1] = c1l
	fe[2] = c2l
	fe[3] = c3l
	fe[4] = c4l

	return fe
}

// Sets fe to a^2.  Returns fe.
func (fe *FieldElement) Square(a *FieldElement) *FieldElement {
	a0 := a[0]
	a1 := a[1]
	a2 := a[2]
	a3 := a[3]
	a4 := a[4]
	a4_38 := a4 * 38

	var carry, h, l uint64

	c0h, c0l := bits.Mul64(a0, a0)
	h, l = bits.Mul64(a4*38, a1)
	c0l, carry = bits.Add64(c0l, l, 0)
	c0h, _ = bits.Add64(c0h, h, carry)
	h, l = bits.Mul64(a2*38, a3)
	c0l, carry = bits.Add64(c0l, l, 0)
	c0h, _ = bits.Add64(c0h, h, carry)

	c1h, c1l := bits.Mul64(2*a0, a1)
	h, l = bits.Mul64(a2, a4_38)
	c1l, carry = bits.Add64(c1l, l, 0)
	c1h, _ = bits.Add64(c1h, h, carry)
	h, l = bits.Mul64(a3*19, a3)
	c1l, carry = bits.Add64(c1l, l, 0)
	c1h, _ = bits.Add64(c1h, h, carry)

	c1l, carry = bits.Add64((c0l>>51)|(c0h<<13), c1l, 0)
	c1h, _ = bits.Add64(c1h, 0, carry)
	c0l &= 0x7ffffffffffff

	c2h, c2l := bits.Mul64(a0, 2*a2)
	h, l = bits.Mul64(a1, a1)
	c2l, carry = bits.Add64(c2l, l, 0)
	c2h, _ = bits.Add64(c2h, h, carry)
	h, l = bits.Mul64(a4_38, a3)
	c2l, carry = bits.Add64(c2l, l, 0)
	c2h, _ = bits.Add64(c2h, h, carry)

	c2l, carry = bits.Add64((c1l>>51)|(c1h<<13), c2l, 0)
	c2h, _ = bits.Add64(c2h, 0, carry)
	c1l &= 0x7ffffffffffff

	c3h, c3l := bits.Mul64(a0, 2*a3)
	h, l = bits.Mul64(2*a1, a2)
	c3l, carry = bits.Add64(c3l, l, 0)
	c3h, _ = bits.Add64(c3h, h, carry)
	h, l = bits.Mul64(a4*19, a4)
	c3l, carry = bits.Add64(c3l, l, 0)
	c3h, _ = bits.Add64(c3h, h, carry)

	c3l, carry = bits.Add64((c2l>>51)|(c2h<<13), c3l, 0)
	c3h, _ = bits.Add64(c3h, 0, carry)
	c2l &= 0x7ffffffffffff

	c4h, c4l := bits.Mul64(a0, 2*a4)
	h, l = bits.Mul64(a2, a2)
	c4l, carry = bits.Add64(c4l, l, 0)
	c4h, _ = bits.Add64(c4h, h, carry)
	h, l = bits.Mul64(a3, 2*a1)
	c4l, carry = bits.Add64(c4l, l, 0)
	c4h, _ = bits.Add64(c4h, h, carry)

	c4l, carry = bits.Add64((c3l>>51)|(c3h<<13), c4l, 0)
	c4h, _ = bits.Add64(c4h, 0, carry)
	c3l &= 0x7ffffffffffff

	carry = ((c4l >> 51) | (c4h << 13))
	c0l += carry * 19
	c4l &= 0x7ffffffffffff
	c1l += c0l >> 51
	c0l &= 0x7ffffffffffff

	fe[0] = c0l
	fe[1] = c1l
	fe[2] = c2l
	fe[3] = c3l
	fe[4] = c4l

	return fe
}

// Sets fe to 2 * a^2.  Returns fe.
func (fe *FieldElement) DoubledSquare(a *FieldElement) *FieldElement {
	a0 := a[0]
	a1 := a[1]
	a2 := a[2]
	a3 := a[3]
	a4 := a[4]
	a0_2 := a0 * 2
	a1_2 := a1 * 2
	a2_2 := a2 * 2
	a3_38 := a3 * 38
	a4_38 := a4 * 38

	var carry, h, l uint64

	c0h, c0l := bits.Mul64(a0, a0_2)
	h, l = bits.Mul64(a4_38, a1_2)
	c0l, carry = bits.Add64(c0l, l, 0)
	c0h, _ = bits.Add64(c0h, h, carry)
	h, l = bits.Mul64(a2_2, a3_38)
	c0l, carry = bits.Add64(c0l, l, 0)
	c0h, _ = bits.Add64(c0h, h, carry)

	c1h, c1l := bits.Mul64(a0_2, a1_2)
	h, l = bits.Mul64(a2_2, a4_38)
	c1l, carry = bits.Add64(c1l, l, 0)
	c1h, _ = bits.Add64(c1h, h, carry)
	h, l = bits.Mul64(a3_38, a3)
	c1l, carry = bits.Add64(c1l, l, 0)
	c1h, _ = bits.Add64(c1h, h, carry)

	c1l, carry = bits.Add64((c0l>>51)|(c0h<<13), c1l, 0)
	c1h, _ = bits.Add64(c1h, 0, carry)
	c0l &= 0x7ffffffffffff

	c2h, c2l := bits.Mul64(a0_2, a2_2)
	h, l = bits.Mul64(a1_2, a1)
	c2l, carry = bits.Add64(c2l, l, 0)
	c2h, _ = bits.Add64(c2h, h, carry)
	h, l = bits.Mul64(a4_38, a3*2)
	c2l, carry = bits.Add64(c2l, l, 0)
	c2h, _ = bits.Add64(c2h, h, carry)

	c2l, carry = bits.Add64((c1l>>51)|(c1h<<13), c2l, 0)
	c2h, _ = bits.Add64(c2h, 0, carry)
	c1l &= 0x7ffffffffffff

	c3h, c3l := bits.Mul64(a0_2, 2*a3)
	h, l = bits.Mul64(a1_2, a2_2)
	c3l, carry = bits.Add64(c3l, l, 0)
	c3h, _ = bits.Add64(c3h, h, carry)
	h, l = bits.Mul64(a4_38, a4)
	c3l, carry = bits.Add64(c3l, l, 0)
	c3h, _ = bits.Add64(c3h, h, carry)

	c3l, carry = bits.Add64((c2l>>51)|(c2h<<13), c3l, 0)
	c3h, _ = bits.Add64(c3h, 0, carry)
	c2l &= 0x7ffffffffffff

	c4h, c4l := bits.Mul64(a0_2, 2*a4)
	h, l = bits.Mul64(a2_2, a2)
	c4l, carry = bits.Add64(c4l, l, 0)
	c4h, _ = bits.Add64(c4h, h, carry)
	h, l = bits.Mul64(2*a3, a1_2)
	c4l, carry = bits.Add64(c4l, l, 0)
	c4h, _ = bits.Add64(c4h, h, carry)

	c4l, carry = bits.Add64((c3l>>51)|(c3h<<13), c4l, 0)
	c4h, _ = bits.Add64(c4h, 0, carry)
	c3l &= 0x7ffffffffffff

	carry = ((c4l >> 51) | (c4h << 13))
	c0l += carry * 19
	c4l &= 0x7ffffffffffff
	c1l += c0l >> 51
	c0l &= 0x7ffffffffffff

	fe[0] = c0l
	fe[1] = c1l
	fe[2] = c2l
	fe[3] = c3l
	fe[4] = c4l

	return fe
}
