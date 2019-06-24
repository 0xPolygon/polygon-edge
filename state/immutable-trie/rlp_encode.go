package itrie

import (
	"encoding/binary"
	"fmt"
)

// Optimized RLP encoding library based on fastjson. It will likely replace minimal/rlp in the future.

type cache struct {
	vs []Value
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

var typeStr = [...]string{
	"array",
	"bytes",
	"null",
	"nullArray",
}

func (t Type) String() string {
	return typeStr[t]
}

// Value is an RLP value
type Value struct {
	a []*Value
	t Type
	b []byte
	l int
}

// Len returns the size of the value
func (v *Value) Len() int {
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

func (a *Arena) NewCopyBytes(b []byte) *Value {
	v := a.c.getValue()
	v.t = TypeBytes
	v.b = extendByteSlice(v.b, len(b))
	copy(v.b, b)
	v.l = len(b)
	return v
}

func (a *Arena) NewBytes(b []byte) *Value {
	v := a.c.getValue()
	v.t = TypeBytes
	v.b = b
	v.l = len(b)
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
		v.l = v.l + intsize(vv.l) + vv.l
	}
	v.a = append(v.a, vv)
}

// This works now because we only use RLP encoding sequentially on the hasher.
// It will need to change if we want to use it as a replacement for the rlp decoder.
var intBuf = make([]byte, 9)

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
	binary.BigEndian.PutUint64(intBuf[1:], uint64(v.l))

	intBuf[8-intSize] = long + byte(intSize)
	return append(dst, intBuf[8-intSize:]...)
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
	valueArrayNull = &Value{t: TypeArrayNull}
	valueNull      = &Value{t: TypeNull}
)

func intsize(val int) int {
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
