package types

import (
	"encoding/json"
	"reflect"

	"github.com/0xPolygon/minimal/helper/hex"
)

var (
	bigT      = reflect.TypeOf((*Big)(nil))
	uint64T   = reflect.TypeOf(Uint64(0))
	hexBytesT = reflect.TypeOf(HexBytes([]byte{}))
)

func isString(input []byte) bool {
	return len(input) >= 2 && input[0] == '"' && input[len(input)-1] == '"'
}

func errNonString(typ reflect.Type) error {
	return &json.UnmarshalTypeError{Value: "non-string", Type: typ}
}

func wrapTypeError(err error, typ reflect.Type) error {
	if _, ok := err.(*hex.DecError); ok {
		return &json.UnmarshalTypeError{Value: err.Error(), Type: typ}
	}
	return err
}

func bytesHave0xPrefix(input []byte) bool {
	return len(input) >= 2 && input[0] == '0' && (input[1] == 'x' || input[1] == 'X')
}

func checkNumberText(input []byte) (raw []byte, err error) {
	if len(input) == 0 {
		return nil, nil // empty strings are allowed
	}
	if !bytesHave0xPrefix(input) {
		return nil, hex.ErrMissingPrefix
	}
	input = input[2:]
	if len(input) == 0 {
		return nil, hex.ErrEmptyNumber
	}
	if len(input) > 1 && input[0] == '0' {
		return nil, hex.ErrLeadingZero
	}
	return input, nil
}
