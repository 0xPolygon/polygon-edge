package buildroot

import (
	"encoding/binary"
	"sync"

	"github.com/0xPolygon/polygon-edge/helper/keccak"
)

func min(i, j int) int {
	if i < j {
		return i
	}

	return j
}

var fastHasherPool sync.Pool

func acquireFastHasher() *FastHasher {
	v := fastHasherPool.Get()
	if v == nil {
		return &FastHasher{k: keccak.NewKeccak256()}
	}

	hasher, ok := v.(*FastHasher)
	if !ok {
		return nil
	}

	return hasher
}

func releaseFastHasher(f *FastHasher) {
	f.reset()
	fastHasherPool.Put(f)
}

// FastHasher is a fast hasher for the derive sha
type FastHasher struct {
	k *keccak.Keccak

	dst  []byte // dst buffer for the outer loop
	buf  []byte // buf for the 16 element group
	buf2 []byte // buf2 to encode auxiliary rlp elements

	num int
	cb  func(i int) []byte
}

func (f *FastHasher) reset() {
	f.k.Reset()
	f.dst = f.dst[:0]
	f.buf = f.buf[:0]
	f.buf2 = f.buf2[:0]
}

func (f *FastHasher) getCb(i int) ([]byte, bool) {
	// bounds check
	if i > f.num {
		return nil, false
	}

	val := f.cb(i)

	if len(val) < 32 {
		return nil, false
	}

	return val, true
}

func (f *FastHasher) encodeOneItem() ([]byte, bool) {
	val, ok := f.getCb(0)
	if !ok {
		return nil, false
	}

	n := getRlpSize(uint64(len(val)))

	f.buf2 = f.marshalListRlpSize(f.buf2[:0], n+len(val)+len(singleKey))
	f.buf2 = append(f.buf2, singleKey...)
	f.buf2 = f.marshalItemRlpSize(f.buf2, len(val))
	f.buf2 = append(f.buf2, val...)

	f.dst = f.hash(f.dst, f.buf2)

	return f.dst, true
}

// Hash hashes the values
func (f *FastHasher) Hash(num int, cb func(i int) []byte) ([]byte, bool) {
	f.cb = cb
	f.num = num

	if num == 1 {
		return f.encodeOneItem()
	}

	var ok bool

	// fill from 0 to 7
	step1 := min(num, 128)
	for i := 0; i < step1; i += 16 {
		ini := i
		if ini == 0 {
			ini = 1
		}

		f.dst = append(f.dst, rlpHashPrefix...)
		f.dst, ok = f.deriveGroup(f.dst, ini, min(step1, i+16))

		if !ok {
			return nil, false
		}
	}

	// fill up the rest of the elements up to 7
	for i := 0; i < (128-step1)/16; i++ {
		f.dst = append(f.dst, rlpEmptyValue...)
	}

	// fill value at 0 in slot 8
	f.dst = append(f.dst, rlpHashPrefix...)

	val, ok := f.getCb(0)
	if !ok {
		return nil, false
	}

	f.dst = f.encodeSingle(f.dst, val, false)

	// fill up from 9 to 16 with empty values
	for i := 9; i <= 16; i++ {
		f.dst = append(f.dst, rlpEmptyValue...)
	}

	f.buf2 = f.marshalListRlpSize(f.buf2[:0], len(f.dst))
	f.buf2 = append(f.buf2, f.dst...)

	f.dst = f.hash(f.dst[:0], f.buf2)

	return f.dst, true
}

func (f *FastHasher) hash(dst, b []byte) []byte {
	f.k.Write(b)
	dst = f.k.Sum(dst)
	f.k.Reset()

	return dst
}

func (f *FastHasher) encodeSingle(dst, v []byte, b bool) []byte {
	num := len(v)
	n := getRlpSize(uint64(num))

	// encode the list rlp-header
	f.buf2 = f.marshalListRlpSize(f.buf2[:0], n+num+1)

	// encode the key (too short to have an rlp header)
	if b {
		f.buf2 = append(f.buf2, oneKey...)
	} else {
		f.buf2 = append(f.buf2, zeroKey...)
	}

	// encode the item rlp-header
	f.buf2 = f.marshalItemRlpSize(f.buf2, num)

	// write the value
	f.buf2 = append(f.buf2, v...)

	return f.hash(dst, f.buf2)
}

// key for a leaf value in a full node
var leafVal = []byte{0x20}

// rlp empty value
var rlpEmptyValue = []byte{0x80}

// rlp encoding prefix for a 32 bytes hash
var rlpHashPrefix = []byte{0xa0}

// key of the zero element
var zeroKey = []byte{0x30}

// key for the one element
var oneKey = []byte{0x31}

// key for a single element
var singleKey = []byte{0x82, 0x20, 0x80}

func (f *FastHasher) deriveGroup(dst []byte, from, to int) ([]byte, bool) {
	f.buf = f.buf[:0]

	num := to - from
	if num == 1 {
		// only one value
		val, ok := f.getCb(from)
		if !ok {
			return nil, false
		}

		if from == 1 {
			dst = f.encodeSingle(dst, val, true)
		} else {
			dst = f.encodeSingle(dst, val, false)
		}

		return dst, true
	}

	if from == 1 {
		// The first group starts at position 1 so we fill position 0
		// with an empty value
		f.buf = append(f.buf, rlpEmptyValue...)
	}

	for i := from; i < to; i++ {
		val, ok := f.getCb(i)
		if !ok {
			return nil, false
		}

		f.buf = append(f.buf, rlpHashPrefix...)

		n := getRlpSize(uint64(len(val)))

		// encode the rlp list
		f.buf2 = f.marshalListRlpSize(f.buf2[:0], n+1+len(val))

		// encode the key
		f.buf2 = append(f.buf2, leafVal...)

		// encode the rlp item
		f.buf2 = f.marshalItemRlpSize(f.buf2, len(val))

		// encode the value
		f.buf2 = append(f.buf2, val...)

		f.buf = f.hash(f.buf, f.buf2)
	}

	// write any other children left up to 16
	if num < 16 {
		last := 15

		if from != 1 {
			last = 16
		}

		for i := num; i < last; i++ {
			f.buf = append(f.buf, rlpEmptyValue...)
		}
	}

	// add an extra empty value which is the leaf of this node
	f.buf = append(f.buf, rlpEmptyValue...)

	f.buf2 = f.marshalListRlpSize(f.buf2[:0], len(f.buf))
	f.buf2 = append(f.buf2, f.buf...)

	dst = f.hash(dst, f.buf2)

	return dst, true
}

func (f *FastHasher) marshalItemRlpSize(dst []byte, size int) []byte {
	return f.marshalRlpSize(dst, uint64(size), 0x80, 0xB7)
}

func (f *FastHasher) marshalListRlpSize(dst []byte, size int) []byte {
	return f.marshalRlpSize(dst, uint64(size), 0xC0, 0xF7)
}

func getRlpSize(size uint64) int {
	if size < 56 {
		return 1
	}

	return int(intsize(size)) + 1
}

func (f *FastHasher) marshalRlpSize(dst []byte, size uint64, short, long byte) []byte {
	if size < 56 {
		return append(dst, short+byte(size))
	}

	buf := make([]byte, 8)
	intSize := intsize(size)

	binary.BigEndian.PutUint64(buf[:], size)

	dst = append(dst, long+byte(intSize))
	dst = append(dst, buf[8-intSize:]...)

	return dst
}

func intsize(val uint64) uint64 {
	switch {
	case val < (1 << 8):
		return 1
	case val < (1 << 16):
		return 2
	case val < (1 << 24):
		return 3
	case val < (1 << 32):
		return 4
	case val < (1 << 40):
		return 5
	case val < (1 << 48):
		return 6
	case val < (1 << 56):
		return 7
	}

	return 8
}
