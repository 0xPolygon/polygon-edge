package fastrlp

import (
	"reflect"

	fuzz "github.com/google/gofuzz"
)

type FuzzObject interface {
	Marshaler
	Unmarshaler
}

type FuzzError struct {
	Source, Target interface{}
}

func (f *FuzzError) Error() string {
	return "failed to encode fuzz object"
}

func Fuzz(num int, obj FuzzObject) error {
	fuzzImpl := func() error {
		f := fuzz.New()
		f.Fuzz(obj)

		data, err := obj.MarshalRLPTo(nil)
		if err != nil {
			return err
		}
		obj2 := reflect.New(reflect.TypeOf(obj).Elem()).Interface().(FuzzObject)
		if err := obj2.UnmarshalRLP(data); err != nil {
			return err
		}
		if !reflect.DeepEqual(obj, obj2) {
			return &FuzzError{Source: obj, Target: obj2}
		}
		return nil
	}

	for i := 0; i < num; i++ {
		if err := fuzzImpl(); err != nil {
			return err
		}
	}
	return nil
}
