package rlp

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"reflect"
	"strings"
)

func randomInt(min, max int) int {
	return min + rand.Intn(max-min)
}

const maxDepth = 7

func generateRandom() interface{} {
	return generateRandomImpl(0)
}

func generateRandomImpl(depth int) interface{} {
	n := randomInt(1, 10)
	if depth == maxDepth {
		n = 10 // force item
	}
	if n < 7 {
		// list
		list := []interface{}{}
		for i := 0; i < randomInt(0, 5); i++ {

			var elem interface{}
			// 10% chance of having a nil value
			if randomInt(0, 10) == 1 {
				elem = nil
			} else {
				elem = generateRandomImpl(depth + 1)
			}

			list = append(list, elem)
		}
		return list
	}
	// item
	b := make([]byte, randomInt(0, 100))
	rand.Read(b)
	return b
}

type kind int

const (
	uintT kind = iota
	boolT
	stringT
	dynamicBytesT
	fixedBytesT
	arrayT
	structT
)

type typ struct {
	kind  kind
	elem  *typ
	size  int
	elems []*typ
}

func (t *typ) Type() reflect.Type {
	return reflect.TypeOf(generateRandomType(t))
}

var randomTypes = []string{
	"uint",
	"bool",
	"string",
	"dynamicbytes",
	"fixedbytes",
	"array",
	"struct",
}

func randomNumberBits() int {
	return randomInt(1, 31) * 8
}

func pickRandomType(d int) *typ {
PICK:
	t := randomTypes[rand.Intn(len(randomTypes))]

	switch t {
	case "uint":
		return &typ{kind: uintT, size: randomNumberBits()}
	case "bool":
		return &typ{kind: boolT}
	case "string":
		return &typ{kind: stringT}
	case "dynamicbytes":
		return &typ{kind: dynamicBytesT}
	case "fixedbytes":
		return &typ{kind: fixedBytesT}
	}

	if d > 3 {
		// Allow only for 3 levels of depth
		goto PICK
	}

	r := pickRandomType(d + 1)
	switch t {
	case "array":
		return &typ{kind: arrayT, elem: r, size: randomInt(1, 3)}

	case "struct":
		size := randomInt(1, 5)
		elems := []*typ{}
		for i := 0; i < size; i++ {
			elems = append(elems, pickRandomType(d+1))
		}
		return &typ{kind: structT, elems: elems}

	default:
		panic(fmt.Errorf("type not implemented: %s", t))
	}
}

var (
	uint8T  = reflect.TypeOf(uint8(0))
	uint16T = reflect.TypeOf(uint16(0))
	uint32T = reflect.TypeOf(uint32(0))
	uint64T = reflect.TypeOf(uint64(0))
)

func mustDecode(str string) []byte {
	if strings.HasPrefix(str, "0x") {
		str = str[2:]
	}
	b, err := hex.DecodeString(str)
	if err != nil {
		panic(err)
	}
	return b
}

func generateNumber(t *typ) interface{} {
	b := make([]byte, t.size/8)
	rand.Read(b)

	var typ reflect.Type
	switch t.size {
	case 8:
		typ = uint8T
	case 16:
		typ = uint16T
	case 32:
		typ = uint32T
	case 64:
		typ = uint64T
	}

	num := big.NewInt(1).SetBytes(b)
	if t.size == 8 || t.size == 16 || t.size == 32 || t.size == 64 {
		return reflect.ValueOf(num.Int64()).Convert(typ).Interface()
	}
	return num
}

func generateRandomType(t *typ) interface{} {

	switch t.kind {
	case uintT:
		return generateNumber(t)

	case boolT:
		if randomInt(0, 2) == 1 {
			return true
		}
		return false

	case stringT:
		return randString(randomInt(1, 100), dictLetters)

	case fixedBytesT:
		return mustDecode(randString(32, hexLetters))

	case dynamicBytesT:
		buf := make([]byte, randomInt(1, 100))
		rand.Read(buf)
		return buf

	case arrayT:
		size := randomInt(0, 5)
		val := reflect.MakeSlice(reflect.SliceOf(t.elem.Type()), size, size)

		for i := 0; i < size; i++ {
			val.Index(i).Set(reflect.ValueOf(generateRandomType(t.elem)))
		}
		return val.Interface()

	case structT:
		fields := []reflect.StructField{}
		for indx, elem := range t.elems {
			fields = append(fields, reflect.StructField{
				Name: fmt.Sprintf("Arg%d", indx),
				Type: elem.Type(),
			})
		}

		typ := reflect.StructOf(fields)
		v := reflect.New(typ)

		for i := 0; i < v.Elem().NumField(); i++ {
			v.Elem().Field(i).Set(reflect.ValueOf(generateRandomType(t.elems[i])))
		}
		return v.Interface()

	default:
		panic(fmt.Errorf("type not implemented: %d", t.kind))
	}
}

const hexLetters = "0123456789abcdef"

const dictLetters = "abcdefghijklmenopqrstuvwxyz"

func randString(n int, dict string) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = dict[rand.Intn(len(dict))]
	}
	return string(b)
}
