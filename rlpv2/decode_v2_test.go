package rlpv2

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/umbracle/minimal/rlp"
)

func compare(v interface{}, val *Value) error {
	return compareImpl(reflect.ValueOf(v), val)
}

func compareImpl(v reflect.Value, val *Value) error {
	if v.Kind() == reflect.Array && isByte(v.Type().Elem()) {
		goto BYTES
	}
	if v.Kind() == reflect.Slice && isByte(v.Type().Elem()) {
		goto BYTES
	}

	if v.Kind() == reflect.Interface {
		return compareImpl(v.Elem(), val)
	}
	if v.Kind() == reflect.Slice || v.Kind() == reflect.Array || v.Kind() == reflect.Struct {
		// val should be array
		if val.t != TypeArray {
			return fmt.Errorf("array expected but found %s", val.Type())
		}

		if v.Kind() == reflect.Struct {
			if v.NumField() != val.Elems() {
				return fmt.Errorf("elems dont match, expected %d but found %d", val.Elems(), v.NumField())
			}
			for i := 0; i < v.NumField(); i++ {
				if err := compareImpl(v.Field(i), val.Get(i)); err != nil {
					return err
				}
			}
		} else {
			if v.Len() != val.Elems() {
				return fmt.Errorf("elems dont match, expected %d but found %d", val.Elems(), v.Len())
			}
			for i := 0; i < v.Len(); i++ {
				if err := compareImpl(v.Index(i), val.Get(i)); err != nil {
					return err
				}
			}
		}
		return nil
	}

BYTES:
	if val.t != TypeBytes {
		return fmt.Errorf("bytes expected but found %s", val.Type())
	}

	switch v.Kind() {
	case reflect.Slice:
		if !bytes.Equal(val.b, v.Bytes()) {
			return fmt.Errorf("bytes dont match")
		}
	default:
		return fmt.Errorf("type not supported '%s'", v.Kind())
	}
	return nil
}

func TestDecode(t *testing.T) {
	for i := 0; i < 10000; i++ {
		t.Run("", func(t *testing.T) {
			tt := pickRandomType(0)
			input := generateRandomType(tt)

			data, err := rlp.EncodeToBytes(input)
			if err != nil {
				t.Fatal(err)
			}

			v2 := reflect.New(tt.Type()).Interface()
			if err := rlp.Decode(data, &v2); err != nil {
				t.Fatal(err)
			}

			p := Parser{}
			v, err := p.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			if err := compare(v2, v); err != nil {
				t.Fatal(err)
			}
		})
	}
}
