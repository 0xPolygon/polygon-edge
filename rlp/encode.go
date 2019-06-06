package rlp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"sync"
)

func EncodeToReader(val interface{}) (int, io.Reader, error) {
	enc := AcquireEncoder()
	defer ReleaseEncoder(enc)

	if err := enc.Encode(val); err != nil {
		return 0, nil, err
	}
	buf := enc.CopyBytes()
	return len(buf), bytes.NewReader(buf), nil
}

func Encode(w io.Writer, val interface{}) error {
	enc := AcquireEncoder()
	defer ReleaseEncoder(enc)

	if err := enc.Encode(val); err != nil {
		return err
	}
	w.Write(enc.Bytes())
	return nil
}

func EncodeToBytes(val interface{}) ([]byte, error) {
	enc := AcquireEncoder()
	defer ReleaseEncoder(enc)

	if err := enc.Encode(val); err != nil {
		return nil, err
	}
	return enc.CopyBytes(), nil
}

var emptyList = []byte{0xC0}

// Value represents any RLP value
type Value struct {
	a    []*Value
	buf  []byte
	size uint
}

func encodeListHeaderSize(size uint) uint {
	if size < 56 {
		return 1
	}
	return uint(intsize(uint64(size))) + 1
}

func (v *Value) encodeHeaderSize() uint {
	size := uint(len(v.buf))
	v.size = size

	slice := v.buf

	if size == 1 && slice[0] <= 0x7F {
		v.size = 0
		return 0
	} else if size < 56 {
		return 1
	} else {
		return uint(intsize(uint64(size))) + 1
	}
}

type Encoder struct {
	cache  []Value
	buf    []byte
	i      uint
	intBuf [9]byte
}

func (e *Encoder) Size() int {
	return len(e.buf)
}

func (e *Encoder) reset() {
	e.i = 0
	e.cache = e.cache[:0]
	e.buf = e.buf[:0]

	for i := 0; i < 9; i++ {
		e.intBuf[i] = 0
	}
}

func (e *Encoder) getValue() *Value {
	if cap(e.cache) > len(e.cache) {
		e.cache = e.cache[:len(e.cache)+1]
	} else {
		e.cache = append(e.cache, Value{})
	}

	v := &e.cache[len(e.cache)-1]
	v.a = v.a[:0]
	v.buf = v.buf[:0]
	v.size = 0

	return v
}

var encPool = sync.Pool{
	New: func() interface{} {
		return new(Encoder)
	},
}

func AcquireEncoder() *Encoder {
	return encPool.Get().(*Encoder)
}

func ReleaseEncoder(enc *Encoder) {
	enc.reset()
	encPool.Put(enc)
}

func (e *Encoder) Encode(v interface{}) error {
	val, size, err := encode(reflect.ValueOf(v), e)
	if err != nil {
		return err
	}

	size = size + uint(len(val.buf))
	e.buf = extendByteSlice(e.buf, int(size))

	e.finishEncoding(val)
	return nil
}

func (e *Encoder) Reset() {
	e.reset()
}

func (e *Encoder) CopyBytes() []byte {
	buf := make([]byte, len(e.buf))
	copy(buf[:], e.buf[:])
	return buf
}

func (e *Encoder) Bytes() []byte {
	return e.buf
}

func (e *Encoder) writeByte(b byte) {
	e.buf[e.i] = b
	e.i++
}

func (e *Encoder) write(b []byte) {
	copy(e.buf[e.i:], b[:])
	e.i += uint(len(b))
}

func (e *Encoder) writeSize(size uint64, short byte, long byte) {
	if size < 56 {
		e.writeByte(short + byte(size))
		return
	}

	intSize := intsize(size)
	binary.BigEndian.PutUint64(e.intBuf[1:], size)

	e.intBuf[8-intSize] = long + byte(intSize)
	e.write(e.intBuf[8-intSize:])
}

func (e *Encoder) finishEncoding(v *Value) {
	// single element without header
	if v.size == 0 {
		e.writeByte(v.buf[0])
		return
	}

	// array
	if len(v.a) != 0 {
		e.writeSize(uint64(v.size), 0xC0, 0xF7)
		for _, elem := range v.a {
			e.finishEncoding(elem)
		}
		return
	}

	// item
	e.writeSize(uint64(v.size), 0x80, 0xB7)
	e.write(v.buf[:])
}

func encode(val reflect.Value, e *Encoder) (*Value, uint, error) {
	kind := val.Type().Kind()

	if kind == reflect.Interface {

		// nil interface returns an empty list
		if val.IsNil() {
			a := e.getValue()
			a.buf = []byte{0xC0}
			return a, 0, nil
		}

		val = val.Elem()
		kind = val.Type().Kind()
	}

	if kind == reflect.Array && isByte(val.Type().Elem()) {
		return encodeBytesArray(val, e)
	}
	if kind == reflect.Slice && isByte(val.Type().Elem()) {
		return encodeBytesSlice(val, e)
	}

	switch kind {
	case reflect.Slice:
		fallthrough
	case reflect.Array:
		return encodeList(val, e)

	case reflect.Bool:
		return encodeBool(val, e)

	case reflect.Struct:
		return encodeStruct(val, e)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return encodeUint(val, e)

	case reflect.String:
		return encodeString(val, e)

	case reflect.Ptr:
		if val.IsNil() {
			v := encodeEmptyValue(e)
			return v, 0, nil
		}

		if val.Type() == bigIntTyp {
			return encodeBigInt(val, e)
		}
		return encode(val.Elem(), e)

	default:
		return nil, 0, fmt.Errorf("encode for type '%s' not supported", kind)
	}
}

func encodeString(val reflect.Value, e *Encoder) (*Value, uint, error) {
	v := e.getValue()

	slice := []byte(val.String())
	v.buf = extendByteSlice(v.buf, len(slice))
	copy(v.buf[:], slice[:])

	return v, v.encodeHeaderSize(), nil
}

var boolTrue = []byte{0x1}
var boolFalse = []byte{0x80}

func encodeBool(val reflect.Value, e *Encoder) (*Value, uint, error) {
	a := e.getValue()

	if val.Bool() {
		a.buf = boolTrue
	} else {
		a.buf = boolFalse
	}

	return a, 0, nil
}

func encodeBytesArray(val reflect.Value, e *Encoder) (*Value, uint, error) {

	if !val.CanAddr() {
		// REDO
		copy := reflect.New(val.Type()).Elem()
		copy.Set(val)
		val = copy
	}

	size := val.Len()

	slice := val.Slice(0, size).Bytes()

	v := e.getValue()
	v.buf = extendByteSlice(v.buf, len(slice))
	copy(v.buf[:], slice[:])

	return v, v.encodeHeaderSize(), nil
}

func encodeBytesSlice(val reflect.Value, e *Encoder) (*Value, uint, error) {
	v := e.getValue()

	slice := val.Bytes()
	v.buf = extendByteSlice(v.buf, len(slice))
	copy(v.buf[:], slice[:])

	if len(v.buf) == 0 {
		v.buf = []byte{0x80}
		return v, 0, nil
	}

	return v, v.encodeHeaderSize(), nil
}

func encodeList(val reflect.Value, e *Encoder) (*Value, uint, error) {
	a := e.getValue()

	total := uint(0)
	a.a = a.a[:0]

	for i := 0; i < val.Len(); i++ {
		v, size, err := encode(val.Index(i), e)
		if err != nil {
			return nil, 0, err
		}

		total = total + uint(len(v.buf)) + size
		a.a = append(a.a, v)
	}

	if total == 0 {
		a.buf = []byte{0xC0}
		return a, 0, nil
	}

	a.size = total
	headerSize := encodeListHeaderSize(total)

	return a, headerSize + a.size, nil
}

func encodeEmptyValue(e *Encoder) *Value {
	a := e.getValue()
	a.buf = []byte{0x80}
	return a
}

func encodeStruct(val reflect.Value, e *Encoder) (*Value, uint, error) {
	fields, err := buildStruct(val.Type())
	if err != nil {
		return nil, 0, err
	}

	a := e.getValue()

	total := uint(0)
	a.a = a.a[:0]

	for _, i := range fields {
		var v *Value
		var size uint

		elem := val.Field(i.indx)
		if i.nilOk {
			if elem.IsNil() {
				v = encodeEmptyValue(e)
				total = total + 1
				a.a = append(a.a, v)
				continue
			}
		}

		v, size, err := encode(elem, e)
		if err != nil {
			return nil, 0, err
		}

		total = total + uint(len(v.buf)) + size
		a.a = append(a.a, v)
	}

	if total == 0 {
		a.buf = []byte{0xC0}
		return a, 0, nil
	}

	a.size = total
	headerSize := encodeListHeaderSize(total)

	return a, headerSize + a.size, nil
}

func encodeUint(val reflect.Value, e *Encoder) (*Value, uint, error) {
	v := e.getValue()

	i := val.Uint()

	if i == 0 {
		v.buf = []byte{0x80}
		return v, 0, nil

	} else if i < 128 {
		v.buf = []byte{byte(i)}
		return v, 0, nil

	} else {
		s := intsize(i)
		binary.BigEndian.PutUint64(e.intBuf[1:], i)
		v.buf = extendByteSlice(v.buf, s)

		copy(v.buf[:], e.intBuf[9-s:])
	}

	return v, v.encodeHeaderSize(), nil
}

func encodeBigInt(val reflect.Value, e *Encoder) (*Value, uint, error) {
	v := e.getValue()

	i := val.Interface().(*big.Int)
	if i == nil {
		v.buf = []byte{0x80}
		return v, 0, nil
	}

	slice := i.Bytes()

	if len(slice) == 0 {
		v.buf = []byte{0x80}
		return v, 0, nil
	}

	v.buf = extendByteSlice(v.buf, len(slice))
	copy(v.buf[:], slice[:])

	return v, v.encodeHeaderSize(), nil
}

func intsize(val uint64) int {
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
