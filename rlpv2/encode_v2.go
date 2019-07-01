package rlpv2

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"sync"
)

// Optimized RLP encoding library based on fastjson. It will likely replace minimal/rlp in the future.

type ArenaPool struct {
	pool sync.Pool
}

func (ap *ArenaPool) Get() *Arena {
	v := ap.pool.Get()
	if v == nil {
		return &Arena{}
	}
	return v.(*Arena)
}

func (ap *ArenaPool) Put(a *Arena) {
	a.Reset()
	ap.pool.Put(a)
}

var arenaPool = sync.Pool{
	New: func() interface{} {
		return new(Arena)
	},
}

func AcquireArena() *Arena {
	return arenaPool.Get().(*Arena)
}

func ReleaseArena(a *Arena) {
	a.Reset()
	arenaPool.Put(a)
}

// bufPool to convert int to bytes
var bufPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 8)
	},
}

type cache struct {
	buf [8]byte
	vs  []Value
}

func (c *cache) reset() {
	c.vs = c.vs[:0]
}

func (c *cache) getValue() *Value {
	if cap(c.vs) > len(c.vs) {
		c.vs = c.vs[:len(c.vs)+1]
	} else {
		c.vs = append(c.vs, Value{})
	}
	return &c.vs[len(c.vs)-1]
}

type Type int

const (
	TypeArray Type = iota
	TypeBytes
	TypeNull      // 0x80
	TypeArrayNull // 0xC0
)

func (t Type) String() string {
	switch t {
	case TypeArray:
		return "array"
	case TypeBytes:
		return "bytes"
	case TypeNull:
		return "null"
	case TypeArrayNull:
		return "null-array"
	default:
		panic(fmt.Errorf("BUG: unknown Value type: %d", t))
	}
}

// Value is an RLP value
type Value struct {
	a []*Value
	t Type
	b []byte
	l uint64
}

func (v *Value) Type() Type {
	return v.t
}

// Get returns the item at index i in the array
func (v *Value) Get(i int) *Value {
	if i > len(v.a) {
		return nil
	}
	return v.a[i]
}

// Elems returns the number of elements if its an array
func (v *Value) Elems() int {
	return len(v.a)
}

// Len returns the size of the value
func (v *Value) Len() uint64 {
	if v.t == TypeArray {
		return v.l + intsize(v.l)
	}
	return v.l
}

// Arena is an RLP object arena
type Arena struct {
	c cache
}

func (a *Arena) Reset() {
	a.c.reset()
}

func extendByteSlice(b []byte, needLen int) []byte {
	b = b[:cap(b)]
	if n := needLen - cap(b); n > 0 {
		b = append(b, make([]byte, n)...)
	}
	return b[:needLen]
}

func (a *Arena) NewString(s string) *Value {
	return a.NewBytes([]byte(s))
}

func (a *Arena) NewBigInt(b *big.Int) *Value {
	if b == nil {
		return valueNull
	}
	return a.NewBytes(b.Bytes())
}

func (a *Arena) NewCopyBytes(b []byte) *Value {
	v := a.c.getValue()
	v.t = TypeBytes
	v.b = extendByteSlice(v.b, len(b))
	copy(v.b, b)
	v.l = uint64(len(b))
	return v
}

func (a *Arena) NewBool(b bool) *Value {
	if b {
		return valueTrue
	}
	return valueFalse
}

func (a *Arena) NewUint(i uint64) *Value {
	if i == 0 {
		return valueNull
	}

	intSize := intsize(i)
	binary.BigEndian.PutUint64(a.c.buf[:], i)

	v := a.c.getValue()
	v.t = TypeBytes
	v.b = extendByteSlice(v.b, int(intSize))
	copy(v.b, a.c.buf[8-intSize:])
	v.l = intSize

	return v
}

func (a *Arena) NewBytes(b []byte) *Value {
	v := a.c.getValue()
	v.t = TypeBytes
	v.b = b
	v.l = uint64(len(b))
	return v
}

func (a *Arena) NewArray() *Value {
	v := a.c.getValue()
	v.t = TypeArray
	v.a = v.a[:0]
	v.l = 0
	return v
}

func (a *Arena) NewNullArray() *Value {
	return valueArrayNull
}

func (a *Arena) NewNull() *Value {
	return valueNull
}

func (v *Value) Bytes() []byte {
	return v.b
}

func (v *Value) Set(vv *Value) {
	if v == nil || v.t != TypeArray {
		return
	}

	// TODO, Add size with the function v.Len()
	if vv.t == TypeNull || vv.t == TypeArrayNull {
		v.l = v.l + 1
	} else if vv.t == TypeBytes {
		size := vv.l
		if size == 1 && vv.b[0] <= 0x7F {
			v.l = v.l + 1
		} else if size < 56 {
			v.l = v.l + 1 + vv.l
		} else {
			v.l = v.l + 1 + intsize(size) + size
		}
	} else {
		if vv.l < 56 {
			v.l = v.l + 1 + vv.l
		} else {
			v.l = v.l + intsize(vv.l) + 1 + vv.l
		}
	}

	v.a = append(v.a, vv)
}

func (v *Value) marshalLongSize(dst []byte) []byte {
	return v.marshalSize(dst, 0xC0, 0xF7)
}

func (v *Value) marshalShortSize(dst []byte) []byte {
	return v.marshalSize(dst, 0x80, 0xB7)
}

func (v *Value) marshalSize(dst []byte, short, long byte) []byte {
	if v.l < 56 {
		return append(dst, short+byte(v.l))
	}

	intSize := intsize(v.l)

	buf := bufPool.Get().([]byte)
	binary.BigEndian.PutUint64(buf[:], uint64(v.l))

	dst = append(dst, long+byte(intSize))
	dst = append(dst, buf[8-intSize:]...)

	bufPool.Put(buf)
	return dst
}

// MarshalTo appends marshaled v to dst and returns the result.
func (v *Value) MarshalTo(dst []byte) []byte {
	switch v.t {
	case TypeBytes:
		if len(v.b) == 1 && v.b[0] <= 0x7F {
			return append(dst, v.b...)
		}
		dst = v.marshalShortSize(dst)
		return append(dst, v.b...)
	case TypeArray:
		dst = v.marshalLongSize(dst)
		for _, vv := range v.a {
			dst = vv.MarshalTo(dst)
		}
		return dst
	case TypeNull:
		return append(dst, []byte{0x80}...)
	case TypeArrayNull:
		return append(dst, []byte{0xC0}...)
	default:
		panic(fmt.Errorf("BUG: unexpected Value type: %d", v.t))
	}
}

var (
	valueArrayNull = &Value{t: TypeArrayNull, l: 1}
	valueNull      = &Value{t: TypeNull, l: 1}
	valueFalse     = valueNull
	valueTrue      = &Value{t: TypeBytes, b: []byte{0x1}, l: 1}
)

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
