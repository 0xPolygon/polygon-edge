package evm

const bitmapSize = uint(8)

type bitmap struct {
	buf []byte
}

func (b *bitmap) isSet(i uint) bool {
	return b.buf[i/bitmapSize]&(1<<(i%bitmapSize)) != 0
}

func (b *bitmap) set(i uint) {
	b.buf[i/bitmapSize] |= 1 << (i % bitmapSize)
}

func (b *bitmap) reset() {
	for i := range b.buf {
		b.buf[i] = 0
	}

	b.buf = b.buf[:0]
}

func (b *bitmap) setCode(code []byte) {
	codeSize := uint(len(code))
	b.buf = extendByteSlice(b.buf, int(codeSize/bitmapSize+1))

	for i := uint(0); i < codeSize; {
		c := code[i]

		if isPushOp(c) {
			// push op
			i += uint(c - 0x60 + 2)
		} else {
			if c == 0x5B {
				// jumpdest
				b.set(i)
			}
			i++
		}
	}
}

func isPushOp(i byte) bool {
	// From PUSH1 (0x60) to PUSH32(0x7F)
	return i>>5 == 3
}
