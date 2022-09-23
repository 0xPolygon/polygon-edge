package edwards25519

func load8u(in []byte) uint64 {
	var r uint64
	r = uint64(in[0])
	r |= uint64(in[1]) << 8
	r |= uint64(in[2]) << 16
	r |= uint64(in[3]) << 24
	r |= uint64(in[4]) << 32
	r |= uint64(in[5]) << 40
	r |= uint64(in[6]) << 48
	r |= uint64(in[7]) << 56
	return r
}

// Computes the w=5 w-NAF of s and store it into naf.
// naf is assumed to be zero-initialized and the highest three bits of s
// have to be cleared.
func computeScalar5NAF(s *[32]byte, naf *[256]int8) {
	var x [5]uint64
	for i := 0; i < 4; i++ {
		x[i] = load8u(s[i*8 : (i+1)*8])
	}
	pos := uint16(0)
	carry := uint64(0)
	for pos < 256 {
		idx := pos / 64
		bit_idx := pos % 64
		var bit_buf uint64
		if bit_idx < 59 {
			bit_buf = x[idx] >> bit_idx
		} else {
			bit_buf = (x[idx] >> bit_idx) | (x[1+idx] << (64 - bit_idx))
		}
		window := carry + (bit_buf & 31)
		if window&1 == 0 {
			pos += 1
			continue
		}

		if window < 16 {
			carry = 0
			naf[pos] = int8(window)
		} else {
			carry = 1
			naf[pos] = int8(window) - int8(32)
		}

		pos += 5
	}
}

func (p *ExtendedPoint) VarTimeScalarMult(q *ExtendedPoint, s *[32]byte) *ExtendedPoint {
	var lut [8]ExtendedPoint

	// Precomputations
	var dblQ ExtendedPoint
	dblQ.Double(q)
	lut[0].Set(q)
	for i := 1; i < 8; i++ {
		lut[i].Add(&lut[i-1], &dblQ)
	}

	// Compute non-adjacent form of s
	var naf [256]int8
	computeScalar5NAF(s, &naf)

	// Skip the trailing zeroes
	i := 255
	for ; i >= 0; i-- {
		if naf[i] != 0 {
			break
		}
	}

	// Corner-case: s is zero.
	p.SetZero()
	if i == -1 {
		return p
	}

	var pp ProjectivePoint
	var cp CompletedPoint
	for {
		if naf[i] > 0 {
			cp.AddExtended(p, &lut[(naf[i]+1)/2-1])
		} else {
			cp.SubExtended(p, &lut[(1-naf[i])/2-1])
		}

		if i == 0 {
			p.SetCompleted(&cp)
			break
		}

		i -= 1
		pp.SetCompleted(&cp)
		cp.DoubleProjective(&pp)

		// Find next non-zero digit
		for {
			if i == 0 || naf[i] != 0 {
				break
			}
			i -= 1
			pp.SetCompleted(&cp)
			cp.DoubleProjective(&pp)
		}
		p.SetCompleted(&cp)

		if naf[i] == 0 {
			break
		}
	}

	return p
}
