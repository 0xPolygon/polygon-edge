package bitmap

// Bitmap Index 0 is LSB from the first bitmap byte
type Bitmap []byte

func (b *Bitmap) Set(idx uint64) {
	index := idx / 8
	*b = extendByteSlice(*b, int(index)+1)

	bit := uint8(1 << (idx % 8))
	(*b)[idx/8] |= bit
}

func (b *Bitmap) Len() uint64 {
	return uint64(len(*b) * 8)
}

func (b *Bitmap) IsSet(idx uint64) bool {
	if b.Len() <= idx {
		return false
	}

	bit := uint8(1 << (idx % 8))

	return (*b)[idx/8]&bit == bit
}

func extendByteSlice(b []byte, needLen int) []byte {
	// for example if we need to store idx 277 we need 35 bytes
	// But if have slice which length is 5 bytes we need to add additional 30 bytes
	// append function is smart enough to use capacity of slice if needed otherwise it will create new slice
	if n := needLen - len(b); n > 0 {
		b = append(b, make([]byte, n)...)
	}

	return b
}
