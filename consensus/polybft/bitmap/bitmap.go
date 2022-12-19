package bitmap

import "github.com/0xPolygon/polygon-edge/helper/common"

// Bitmap Index 0 is LSB from the first bitmap byte
type Bitmap []byte

func (b *Bitmap) Set(idx uint64) {
	index := idx / 8
	*b = common.ExtendByteSlice(*b, int(index)+1, false)

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
