package contractsapi

import (
	"bytes"
	"fmt"
	"math/big"
	"reflect"

	"github.com/mitchellh/mapstructure"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

const abiMethodIDLength = 4

func decodeEvent(event *abi.Event, log *ethgo.Log, out interface{}) error {
	val, err := event.ParseLog(log)
	if err != nil {
		return err
	}

	return decodeImpl(val, out)
}

func decodeMethod(method *abi.Method, input []byte, out interface{}) error {
	if len(input) < abiMethodIDLength {
		return fmt.Errorf("invalid method data, len = %d", len(input))
	}

	sig := method.ID()
	if !bytes.HasPrefix(input, sig) {
		return fmt.Errorf("prefix is not correct")
	}

	val, err := abi.Decode(method.Inputs, input[abiMethodIDLength:])
	if err != nil {
		return err
	}

	return decodeImpl(val, out)
}

func decodeStruct(t *abi.Type, input []byte, out interface{}) error {
	if len(input) < abiMethodIDLength {
		return fmt.Errorf("invalid struct data, len = %d", len(input))
	}

	val, err := abi.Decode(t, input)
	if err != nil {
		return err
	}

	return decodeImpl(val, out)
}

func decodeImpl(input interface{}, out interface{}) error {
	metadata := &mapstructure.Metadata{}
	dc := &mapstructure.DecoderConfig{
		Result:     out,
		TagName:    "abi",
		Metadata:   metadata,
		DecodeHook: customHook,
	}

	ms, err := mapstructure.NewDecoder(dc)
	if err != nil {
		return err
	}

	if err = ms.Decode(input); err != nil {
		return err
	}

	if len(metadata.Unused) != 0 {
		return fmt.Errorf("some keys not used: %v", metadata.Unused)
	}

	return nil
}

var bigTyp = reflect.TypeOf(new(big.Int))

func customHook(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
	if f == bigTyp && t.Kind() == reflect.Uint64 {
		// convert big.Int to uint64 (if possible)
		b, ok := data.(*big.Int)
		if !ok {
			return nil, fmt.Errorf("data not a big.Int")
		}

		if !b.IsUint64() {
			return nil, fmt.Errorf("cannot format big.Int to uint64")
		}

		return b.Uint64(), nil
	}

	return data, nil
}
