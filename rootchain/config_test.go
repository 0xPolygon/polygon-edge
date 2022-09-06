package rootchain

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRootchainConfig(t *testing.T) {
	const (
		rootchainAddress   = "0x123456"
		eventABI           = "event ABI"
		methodABI          = "method ABI"
		localAddress       = "0x123123"
		payloadType        = ValidatorSetPayloadType
		blockConfirmations = uint64(10)
	)

	config := Config{
		RootchainAddresses: map[string][]ConfigEvent{
			rootchainAddress: {
				{
					EventABI:           eventABI,
					MethodABI:          methodABI,
					LocalAddress:       localAddress,
					PayloadType:        payloadType,
					BlockConfirmations: blockConfirmations,
				},
			},
		},
	}

	// Marshal the data into JSON
	marshalledData, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("unable to marshal rootchain config, %v", err)
	}

	// Unmarshal the data
	destination := reflect.New(reflect.TypeOf(config))
	if err = json.Unmarshal(marshalledData, destination.Interface()); err != nil {
		t.Fatalf("unable to unmarshal rootchain config, %v", err)
	}

	// Make sure the configs are equal
	assert.Equal(t, config, destination.Elem().Interface())
}
