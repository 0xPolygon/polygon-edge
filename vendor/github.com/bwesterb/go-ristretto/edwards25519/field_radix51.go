// +build amd64,!forcegeneric go1.13,!forcegeneric

package edwards25519

// Element of the field GF(2^255 - 19) over which the elliptic
// curve Edwards25519 is defined.
type FieldElement [5]uint64

var (
	feZero     FieldElement
	feOne      = FieldElement{1, 0, 0, 0, 0}
	feMinusOne = FieldElement{2251799813685228, 2251799813685247,
		2251799813685247, 2251799813685247, 2251799813685247}

	// sqrt(-1)
	feI = FieldElement{
		1718705420411056, 234908883556509, 2233514472574048,
		2117202627021982, 765476049583133,
	}

	// -sqrt(-1)
	feMinusI = FieldElement{
		533094393274173, 2016890930128738, 18285341111199,
		134597186663265, 1486323764102114,
	}

	// parameter d of Edwards25519
	feD = FieldElement{
		929955233495203, 466365720129213, 1662059464998953,
		2033849074728123, 1442794654840575,
	}

	// double feD
	fe2D = FieldElement{
		1859910466990425, 932731440258426, 1072319116312658,
		1815898335770999, 633789495995903,
	}

	// 1 / sqrt(-1-d)
	feInvSqrtMinusDMinusOne = FieldElement{
		1972891073822467, 1430154612583622, 2243686579258279,
		473840635492096, 133279003116800,
	}

	// 2 / sqrt(-1-d)
	feDoubleInvSqrtMinusDMinusOne = FieldElement{
		1693982333959686, 608509411481997, 2235573344831311,
		947681270984193, 266558006233600,
	}

	// 2i / sqrt(-1-d)
	feDoubleIInvSqrtMinusDMinusOne = FieldElement{
		1608655899704280, 1999971613377227, 49908634785720,
		1873700692181652, 353702208628067,
	}

	// (d-1)^2
	feDMinusOneSquared = FieldElement{
		1507062230895904, 1572317787530805, 683053064812840,
		317374165784489, 1572899562415810,
	}

	// 1 - d^2
	feOneMinusDSquared = FieldElement{
		1136626929484150, 1998550399581263, 496427632559748,
		118527312129759, 45110755273534,
	}

	// (d+1)/(d-1)
	feDp1OverDm1 = FieldElement{
		2159851467815724, 1752228607624431, 1825604053920671,
		1212587319275468, 253422448836237,
	}

	// sqrt(i*d)
	feSqrtID = FieldElement{
		2298852427963285, 3837146560810661, 4413131899466403,
		3883177008057528, 2352084440532925,
	}

	// 1/sqrt(d+1)
	feInvSqrt1pD = FieldElement{
		321571956990465, 1251814006996634, 2226845496292387,
		189049560751797, 2074948709371214,
	}

	epZero = ExtendedPoint{feZero, feOne, feOne, feZero}

	epBase = ExtendedPoint{
		FieldElement{2678275328304575, 4315672520525287, 2266428086574206,
			2359477563015859, 2540138899492839},
		FieldElement{1934594822876571, 2049809580636559, 1991994783322914,
			1758681962032007, 380046701118659},
		feOne,
		FieldElement{410445769351754, 2235400917701188, 1495825632738689,
			1351628537510093, 430502003771208},
	}
)

// Neg sets fe to -a.  Returns fe.
func (fe *FieldElement) Neg(a *FieldElement) *FieldElement {
	var t FieldElement
	t = *a

	t[1] += t[0] >> 51
	t[0] = t[0] & 0x7ffffffffffff
	t[2] += t[1] >> 51
	t[1] = t[1] & 0x7ffffffffffff
	t[3] += t[2] >> 51
	t[2] = t[2] & 0x7ffffffffffff
	t[4] += t[3] >> 51
	t[3] = t[3] & 0x7ffffffffffff
	t[0] += (t[4] >> 51) * 19
	t[4] = t[4] & 0x7ffffffffffff

	fe[0] = 0xfffffffffffda - t[0]
	fe[1] = 0xffffffffffffe - t[1]
	fe[2] = 0xffffffffffffe - t[2]
	fe[3] = 0xffffffffffffe - t[3]
	fe[4] = 0xffffffffffffe - t[4]

	return fe
}

// Sets fe to a-b. Returns fe.
func (fe *FieldElement) sub(a, b *FieldElement) *FieldElement {
	var t FieldElement
	t = *b

	t[1] += t[0] >> 51
	t[0] = t[0] & 0x7ffffffffffff
	t[2] += t[1] >> 51
	t[1] = t[1] & 0x7ffffffffffff
	t[3] += t[2] >> 51
	t[2] = t[2] & 0x7ffffffffffff
	t[4] += t[3] >> 51
	t[3] = t[3] & 0x7ffffffffffff
	t[0] += (t[4] >> 51) * 19
	t[4] = t[4] & 0x7ffffffffffff

	fe[0] = (a[0] + 0xfffffffffffda) - t[0]
	fe[1] = (a[1] + 0xffffffffffffe) - t[1]
	fe[2] = (a[2] + 0xffffffffffffe) - t[2]
	fe[3] = (a[3] + 0xffffffffffffe) - t[3]
	fe[4] = (a[4] + 0xffffffffffffe) - t[4]

	return fe
}

// Sets fe to a + b without normalizing.  Returns fe.
func (fe *FieldElement) add(a, b *FieldElement) *FieldElement {
	fe[0] = a[0] + b[0]
	fe[1] = a[1] + b[1]
	fe[2] = a[2] + b[2]
	fe[3] = a[3] + b[3]
	fe[4] = a[4] + b[4]
	return fe
}

// Reduce the even coefficients.  Returns fe.
func (fe *FieldElement) normalize() *FieldElement {
	return fe.setReduced(fe)
}

// Set fe to a reduced version of a.  Returns fe.
func (fe *FieldElement) setReduced(a *FieldElement) *FieldElement {
	*fe = *a

	fe[1] += fe[0] >> 51
	fe[0] = fe[0] & 0x7ffffffffffff
	fe[2] += fe[1] >> 51
	fe[1] = fe[1] & 0x7ffffffffffff
	fe[3] += fe[2] >> 51
	fe[2] = fe[2] & 0x7ffffffffffff
	fe[4] += fe[3] >> 51
	fe[3] = fe[3] & 0x7ffffffffffff
	fe[0] += (fe[4] >> 51) * 19
	fe[4] = fe[4] & 0x7ffffffffffff

	c := (fe[0] + 19) >> 51
	c = (fe[1] + c) >> 51
	c = (fe[2] + c) >> 51
	c = (fe[3] + c) >> 51
	c = (fe[4] + c) >> 51

	fe[0] += 19 * c

	fe[1] += fe[0] >> 51
	fe[0] = fe[0] & 0x7ffffffffffff
	fe[2] += fe[1] >> 51
	fe[1] = fe[1] & 0x7ffffffffffff
	fe[3] += fe[2] >> 51
	fe[2] = fe[2] & 0x7ffffffffffff
	fe[4] += fe[3] >> 51
	fe[3] = fe[3] & 0x7ffffffffffff
	fe[4] = fe[4] & 0x7ffffffffffff

	return fe
}

// Set fe to a if b == 1.  Requires b to be either 0 or 1.
func (fe *FieldElement) ConditionalSet(a *FieldElement, b int32) {
	b2 := uint64(1-b) - 1
	fe[0] ^= b2 & (fe[0] ^ a[0])
	fe[1] ^= b2 & (fe[1] ^ a[1])
	fe[2] ^= b2 & (fe[2] ^ a[2])
	fe[3] ^= b2 & (fe[3] ^ a[3])
	fe[4] ^= b2 & (fe[4] ^ a[4])
}

// Write fe to s in little endian.  Returns fe.
func (fe *FieldElement) BytesInto(s *[32]byte) *FieldElement {
	var t FieldElement
	t.setReduced(fe)

	s[0] = byte(t[0] & 0xff)
	s[1] = byte((t[0] >> 8) & 0xff)
	s[2] = byte((t[0] >> 16) & 0xff)
	s[3] = byte((t[0] >> 24) & 0xff)
	s[4] = byte((t[0] >> 32) & 0xff)
	s[5] = byte((t[0] >> 40) & 0xff)
	s[6] = byte((t[0] >> 48))
	s[6] ^= byte((t[1] << 3) & 0xf8)
	s[7] = byte((t[1] >> 5) & 0xff)
	s[8] = byte((t[1] >> 13) & 0xff)
	s[9] = byte((t[1] >> 21) & 0xff)
	s[10] = byte((t[1] >> 29) & 0xff)
	s[11] = byte((t[1] >> 37) & 0xff)
	s[12] = byte((t[1] >> 45))
	s[12] ^= byte((t[2] << 6) & 0xc0)
	s[13] = byte((t[2] >> 2) & 0xff)
	s[14] = byte((t[2] >> 10) & 0xff)
	s[15] = byte((t[2] >> 18) & 0xff)
	s[16] = byte((t[2] >> 26) & 0xff)
	s[17] = byte((t[2] >> 34) & 0xff)
	s[18] = byte((t[2] >> 42) & 0xff)
	s[19] = byte((t[2] >> 50))
	s[19] ^= byte((t[3] << 1) & 0xfe)
	s[20] = byte((t[3] >> 7) & 0xff)
	s[21] = byte((t[3] >> 15) & 0xff)
	s[22] = byte((t[3] >> 23) & 0xff)
	s[23] = byte((t[3] >> 31) & 0xff)
	s[24] = byte((t[3] >> 39) & 0xff)
	s[25] = byte((t[3] >> 47))
	s[25] ^= byte((t[4] << 4) & 0xf0)
	s[26] = byte((t[4] >> 4) & 0xff)
	s[27] = byte((t[4] >> 12) & 0xff)
	s[28] = byte((t[4] >> 20) & 0xff)
	s[29] = byte((t[4] >> 28) & 0xff)
	s[30] = byte((t[4] >> 36) & 0xff)
	s[31] = byte((t[4] >> 44))
	return fe
}

// Sets fe to the little endian number encoded in buf modulo 2^255-19.
// Ignores the highest bit in buf.  Returns fe.
func (fe *FieldElement) SetBytes(buf *[32]byte) *FieldElement {
	fe[0] = (uint64(buf[0]) | (uint64(buf[1]) << 8) | (uint64(buf[2]) << 16) |
		(uint64(buf[3]) << 24) | (uint64(buf[4]) << 32) |
		(uint64(buf[5]) << 40) | (uint64(buf[6]&7) << 48))
	fe[1] = ((uint64(buf[6]) >> 3) | (uint64(buf[7]) << 5) |
		(uint64(buf[8]) << 13) | (uint64(buf[9]) << 21) |
		(uint64(buf[10]) << 29) | (uint64(buf[11]) << 37) |
		(uint64(buf[12]&63) << 45))
	fe[2] = ((uint64(buf[12]) >> 6) | (uint64(buf[13]) << 2) |
		(uint64(buf[14]) << 10) | (uint64(buf[15]) << 18) |
		(uint64(buf[16]) << 26) | (uint64(buf[17]) << 34) |
		(uint64(buf[18]) << 42) | (uint64(buf[19]&1) << 50))
	fe[3] = ((uint64(buf[19]) >> 1) | (uint64(buf[20]) << 7) |
		(uint64(buf[21]) << 15) | (uint64(buf[22]) << 23) |
		(uint64(buf[23]) << 31) | (uint64(buf[24]) << 39) |
		(uint64(buf[25]&15) << 47))
	fe[4] = ((uint64(buf[25]) >> 4) | (uint64(buf[26]) << 4) |
		(uint64(buf[27]) << 12) | (uint64(buf[28]) << 20) |
		(uint64(buf[29]) << 28) | (uint64(buf[30]) << 36) |
		(uint64(buf[31]&127) << 44))
	return fe
}
