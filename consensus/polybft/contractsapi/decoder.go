package contractsapi

import (
	"bytes"
	"fmt"

	"github.com/mitchellh/mapstructure"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

func decodeEvent(event *abi.Event, log *ethgo.Log, out interface{}) error {
	val, err := event.ParseLog(log)
	if err != nil {
		return err
	}

	return decodeImpl(val, out)
}

func decodeMethod(method *abi.Method, input []byte, out interface{}) error {
	if len(input) < 4 {
		return fmt.Errorf("invalid bundle data, len = %d", len(input))
	}

	sig := method.ID()
	if !bytes.HasPrefix(input, sig) {
		return fmt.Errorf("prefix is not correct")
	}

	val, err := abi.Decode(method.Inputs, input[4:])
	if err != nil {
		return err
	}

	return decodeImpl(val, out)
}

func decodeImpl(input interface{}, out interface{}) error {
	metadata := &mapstructure.Metadata{}
	dc := &mapstructure.DecoderConfig{
		Result:   out,
		TagName:  "abi",
		Metadata: metadata,
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
