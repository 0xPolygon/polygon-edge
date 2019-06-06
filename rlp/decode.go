package rlp

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"reflect"
	"strings"
	"sync"
)

var sliceTyp = reflect.TypeOf([]interface{}{})
var bigIntTyp = reflect.TypeOf(new(big.Int))

func extendByteSlice(b []byte, needLen int) []byte {
	b = b[:cap(b)]
	if n := needLen - cap(b); n > 0 {
		b = append(b, make([]byte, n)...)
	}
	return b[:needLen]
}

// Type represents a RLP type.
type Type int

const (
	// TypeList is an RLP list type.
	TypeList Type = 0

	// TypeBytes is an RLP bytes type.
	TypeBytes Type = 1

	// TypeByte is a single byte
	TypeByte Type = 2
)

// Iterator iterates over values
type Iterator struct {
	r io.Reader

	cur   byte
	cache bool

	// buffers
	intb [8]byte
	buf  []byte

	// stack of lists
	lists []int

	readed int
}

func (i *Iterator) Readed() int {
	return i.readed
}

func (i *Iterator) RawBytes() []byte {
	return nil
}

func (i *Iterator) Raw() []byte {
	if len(i.lists) == 0 {
		// return the rest of the string
		data, _ := ioutil.ReadAll(i.r)
		return data
	}

	rest := i.lists[len(i.lists)-1]
	buf := make([]byte, rest)
	i.Read(buf[:])

	return buf
}

func (i *Iterator) consume(n int) error {
	i.readed += n

	if len(i.lists) == 0 {
		return nil
	}

	i.lists[len(i.lists)-1] = i.lists[len(i.lists)-1] - n
	if i.lists[len(i.lists)-1] < 0 {
		return fmt.Errorf("overflow consume")
	}

	return nil
}

func (i *Iterator) isFinished() bool {
	if len(i.lists) == 0 {
		return false
	}
	return i.lists[len(i.lists)-1] == 0
}

func (i *Iterator) uint(bits int) (uint64, error) {
	kind, size, err := i.readKind()
	if err != nil {
		return 0, err
	}

	if kind == TypeByte {
		return uint64(size), nil
	}
	if size > uint64(bits/8) {
		return 0, fmt.Errorf("too many bits to return")
	}
	return i.readUint(uint(size))
}

func (i *Iterator) isZeroNext() (bool, error) {
	cur, err := i.Peek()
	if err != nil {
		return false, err
	}

	isZero := cur == 0x80
	if isZero {
		i.ReadByte()
		i.consume(1)
	}

	return isZero, nil
}

func (i *Iterator) IsListNext() (bool, error) {
	cur, err := i.Peek()
	if err != nil {
		return false, err
	}
	return cur >= 0xC0, nil
}

func (i *Iterator) Peek() (byte, error) {
	cur, err := i.ReadByte()
	i.cache = true
	return cur, err
}

func (i *Iterator) ReadByte() (byte, error) {
	if i.cache {
		i.cache = false
		return i.cur, nil
	}

	b := [1]byte{}
	if _, err := i.r.Read(b[:]); err != nil {
		return 0, err
	}

	i.cur = b[0]
	return i.cur, nil
}

func (i *Iterator) readKind() (Type, uint64, error) {
	cur, err := i.ReadByte()
	if err != nil {
		return TypeByte, 0, err
	}

	// consume one byte in the list
	i.consume(1)

	if cur < 0x80 {
		// single byte object
		return TypeByte, uint64(cur), err

	} else if cur < 0xB8 {
		// item less 55 bytes long
		return TypeBytes, uint64(cur - 0x80), err

	} else if cur < 0xC0 {
		// item longer than 55 bytes
		size, err := i.readUint(uint(cur - 0xB7))
		if err == nil && size < 56 {
			err = fmt.Errorf("bad size")
		}
		return TypeBytes, size, err

	} else if cur < 0xF8 {
		// list less than 55 bytes longer
		return TypeList, uint64(cur - 0xC0), err
	}

	size, err := i.readUint(uint(cur - 0xf7))
	if err == nil && size < 56 {
		err = fmt.Errorf("bad size")
	}

	return TypeList, size, err
}

func (i *Iterator) Read(buf []byte) error {
	total := 0
	for total < len(buf) {
		n, err := i.r.Read(buf[total:])
		total += n
		if err != nil {
			return err
		}
	}

	if err := i.consume(total); err != nil {
		return err
	}
	return nil
}

// Discard moves the pointer of the buffer to the end
func (i *Iterator) Discard() error {
	n := i.lists[len(i.lists)-1]
	i.lists[len(i.lists)-1] = 0

	// discard the next n values
	_, err := io.CopyN(ioutil.Discard, i.r, int64(n))
	return err
}

func (i *Iterator) BytesX(buf []byte) error {
	typ, _, err := i.readKind()
	if err != nil {
		return err
	}
	if typ == TypeList {
		return fmt.Errorf("list not expected")
	}

	if err := i.Read(buf); err != nil {
		return err
	}
	return nil
}

func (i *Iterator) Bytes() ([]byte, error) {
	typ, size, err := i.readKind()
	if err != nil {
		return nil, err
	}
	if typ == TypeList {
		return nil, fmt.Errorf("list not expected")
	}
	if typ == TypeByte {
		return []byte{byte(size)}, nil
	}

	// check size overflow here
	if size > 0xffffffffe0 {
		return nil, fmt.Errorf("overflow")
	}

	i.buf = extendByteSlice(i.buf, int(size))
	if err := i.Read(i.buf); err != nil {
		return nil, err
	}

	if size == 1 && i.buf[0] < 128 {
		return nil, fmt.Errorf("F")
	}
	return i.buf, nil
}

func (i *Iterator) readUint(size uint) (uint64, error) {
	for j := 0; j < 8; j++ {
		i.intb[j] = 0
	}
	if err := i.Read(i.intb[8-size:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(i.intb[:]), nil
}

func (i *Iterator) Count() (int, error) {
	_, err := i.List()
	if err != nil {
		return 0, err
	}

	c := 0
	for {
		isList, err := i.IsListNext()
		if err != nil {
			return 0, err
		}

		if isList {
			i.List()
			if err := i.Discard(); err != nil {
				return 0, err
			}
			i.ListEnd()
		} else {
			if _, err := i.Bytes(); err != nil {
				return 0, err
			}
		}

		c++
		if i.isFinished() {
			break
		}
	}
	return c, nil
}

// List starts a list iteration
func (i *Iterator) List() (uint64, error) {
	typ, size, err := i.readKind()
	if err != nil {
		return 0, err
	}

	if typ != TypeList {
		return 0, fmt.Errorf("list expected")
	}

	if size != 0 {
		if err := i.consume(int(size)); err != nil {
			return 0, err
		}
		i.lists = append(i.lists, int(size))
	}
	return size, nil
}

// ListEnd ends the list iteration
func (i *Iterator) ListEnd() error {
	size := len(i.lists)
	if size == 0 {
		return fmt.Errorf("no list to popup")
	}

	rest := i.lists[size-1]
	if rest != 0 {
		return fmt.Errorf("bad rest %d", rest)
	}

	// pop up
	i.lists = i.lists[:size-1]
	return nil
}

func NewIterator(b []byte) *Iterator {
	return &Iterator{
		r:     bytes.NewReader(b),
		lists: []int{},
	}
}

func DecodeBytes(b []byte, val interface{}) error {
	return Decode(b, val)
}

func DecodeReader(b io.Reader, val interface{}) error {
	i := &Iterator{
		r:     b,
		lists: []int{},
	}

	v := reflect.ValueOf(val)
	if v.Type().Kind() != reflect.Ptr {
		return fmt.Errorf("it must be a pointer")
	}
	if v.IsNil() {
		return fmt.Errorf("the pointer cannot be nil")
	}

	return decode(i, v.Elem())
}

// Decode decodes an rlp value
func Decode(b []byte, val interface{}) error {
	i := &Iterator{
		r:     bufio.NewReader(bytes.NewReader(b)),
		lists: []int{},
	}

	v := reflect.ValueOf(val)
	if v.Type().Kind() != reflect.Ptr {
		return fmt.Errorf("it must be a pointer")
	}
	if v.IsNil() {
		return fmt.Errorf("the pointer cannot be nil")
	}

	return decode(i, v.Elem())
}

func decode(i *Iterator, val reflect.Value) error {
	typ := val.Type()

	if typ.Kind() == reflect.Array && isByte(val.Type().Elem()) {
		return decodeBytesArray(i, val)
	}
	if typ.Kind() == reflect.Slice && isByte(val.Type().Elem()) {
		return decodeBytesSlice(i, val)
	}

	switch typ.Kind() {
	case reflect.Slice:
		fallthrough
	case reflect.Array:
		return decodeList(i, val)

	case reflect.Struct:
		return decodeStruct(i, val)

	case reflect.String:
		return decodeString(i, val)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return decodeUint(i, val)

	case reflect.Bool:
		return decodeBool(i, val)

	case reflect.Ptr:
		if val.Type() == bigIntTyp {
			return decodeBigInt(i, val)
		}

		newval := val
		if val.IsNil() {
			newval = reflect.New(typ.Elem())
		}
		if err := decode(i, newval.Elem()); err != nil {
			return err
		}
		val.Set(newval)
		return nil

	case reflect.Interface:
		return decodeInterface(i, val)

	default:
		return fmt.Errorf("decode for type '%s' not supported", typ.Kind())
	}
}

func isByte(typ reflect.Type) bool {
	return typ.Kind() == reflect.Uint8
}

func decodeBool(i *Iterator, val reflect.Value) error {
	num, err := i.uint(8)
	if err != nil {
		return err
	}
	if num == 0 {
		val.SetBool(false)
	} else if num == 1 {
		val.SetBool(true)
	} else {
		return fmt.Errorf("invalid bool")
	}
	return nil
}

func decodeString(i *Iterator, val reflect.Value) error {
	b, err := i.Bytes()
	if err != nil {
		return err
	}
	val.SetString(string(b))
	return nil
}

func decodeUint(i *Iterator, val reflect.Value) error {
	typ := val.Type()

	num, err := i.uint(typ.Bits())
	if err != nil {
		return err
	}
	val.SetUint(num)
	return nil
}

func decodeBigInt(i *Iterator, val reflect.Value) error {
	b, err := i.Bytes()
	if err != nil {
		return err
	}
	ii := val.Interface().(*big.Int)
	if ii == nil {
		ii = new(big.Int)
		val.Set(reflect.ValueOf(ii))
	}
	ii.SetBytes(b)
	return nil
}

func decodeInterface(i *Iterator, val reflect.Value) error {
	isListNext, err := i.IsListNext()
	if err != nil {
		return err
	}

	if isListNext {
		slice := reflect.New(sliceTyp).Elem()
		if err := decodeList(i, slice); err != nil {
			return err
		}
		val.Set(slice)
	} else {
		buf, err := i.Bytes()
		if err != nil {
			return err
		}

		aux := make([]byte, len(buf))
		copy(aux[:], buf[:])

		val.Set(reflect.ValueOf(aux))
	}
	return nil
}

func decodeBytesSlice(i *Iterator, val reflect.Value) error {
	buf, err := i.Bytes()
	if err != nil {
		return err
	}

	aux := make([]byte, len(buf))
	copy(aux[:], buf[:])

	val.SetBytes(aux)
	return nil
}

func decodeBytesArray(i *Iterator, val reflect.Value) error {
	valBuf := val.Slice(0, val.Len()).Interface().([]byte)
	if err := i.BytesX(valBuf[:]); err != nil {
		return err
	}
	return nil
}

type field struct {
	indx  int
	tail  bool
	nilOk bool
}

var structLock sync.Mutex
var structCache map[reflect.Type][]*field

func init() {
	structCache = map[reflect.Type][]*field{}
}

func buildStruct(typ reflect.Type) ([]*field, error) {

	structLock.Lock()
	defer structLock.Unlock()

	if elem, ok := structCache[typ]; ok {
		return elem, nil
	}

	fields := []*field{}
	num := typ.NumField()
	for i := 0; i < num; i++ {
		if f := typ.Field(i); f.PkgPath == "" {
			elem := &field{
				indx: i,
			}

			ignore := false
			for _, tag := range strings.Split(f.Tag.Get("rlp"), ",") {
				tag = strings.TrimSpace(tag)

				switch tag {
				case "tail":
					if num != i+1 {
						return nil, fmt.Errorf("tail is only valid as the last element of the struct")
					}
					elem.tail = true

				case "nil":
					if f.Type.Kind() != reflect.Ptr {
						return nil, fmt.Errorf("rlp nil only possible with ptr")
					}
					elem.nilOk = true

				case "-":
					ignore = true
				}
			}

			if !ignore {
				fields = append(fields, elem)
			}
		}
	}

	structCache[typ] = fields
	return fields, nil
}

func decodeStruct(i *Iterator, val reflect.Value) error {
	fields, err := buildStruct(val.Type())
	if err != nil {
		return err
	}

	if _, err := i.List(); err != nil {
		return err
	}

	for _, j := range fields {
		elem := val.Field(j.indx)

		if j.nilOk {
			isZero, err := i.isZeroNext()
			if err != nil {
				return err
			}
			if isZero {
				elem.Set(reflect.Zero(elem.Type()))
				continue
			}
		}

		if err := decode(i, elem); err != nil {
			return err
		}
		if j.tail {
			if err := i.Discard(); err != nil {
				return err
			}
		}
	}

	if err := i.ListEnd(); err != nil {
		return err
	}
	return nil
}

func decodeList(i *Iterator, slice reflect.Value) error {
	typ := slice.Type()

	size, err := i.List()
	if err != nil {
		return err
	}
	if size == 0 {
		slice.Set(reflect.MakeSlice(typ, 0, 0))
		return nil
	}

	var elem reflect.Value
	var j int

BACK:
	slice, elem = expand(slice, typ, j)

	if err := decode(i, elem); err != nil {
		return err
	}
	if !i.isFinished() {
		j++
		goto BACK
	}

	if err := i.ListEnd(); err != nil {
		return err
	}
	return nil
}

func expand(slice reflect.Value, typ reflect.Type, total int) (reflect.Value, reflect.Value) {
	cap := slice.Cap()
	len := slice.Len()

	if total >= cap {
		newSize := total*3/2 + 1
		newslice := reflect.MakeSlice(typ, len, newSize)
		reflect.Copy(newslice, slice)
		slice.Set(newslice)
	}

	if total >= slice.Len() {
		slice.SetLen(total + 1)
	}
	return slice, slice.Index(total)
}
