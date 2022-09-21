package samuel

import (
	"github.com/0xPolygon/polygon-edge/rootchain"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/abi"
)

// eventData holds information on event data mapping
type eventData struct {
	payloadType  rootchain.PayloadType
	eventABI     *abi.Event
	methodABI    *abi.Method
	localAddress types.Address
}

// newEventData generates the SAMUEL event data lookup map from the
// passed in rootchain configuration
func newEventData(
	configEvent *rootchain.ConfigEvent,
) eventData {
	return eventData{
		payloadType:  configEvent.PayloadType,
		eventABI:     abi.MustNewEvent(configEvent.EventABI),
		methodABI:    abi.MustNewABI(configEvent.MethodABI).GetMethod(configEvent.MethodName),
		localAddress: types.StringToAddress(configEvent.LocalAddress),
	}
}

// getMethodID returns the method ID
func (e eventData) getMethodID() []byte {
	return e.methodABI.ID()
}

// getLocalAddress returns the local SC address
// as a string representation
func (e eventData) getLocalAddress() string {
	return e.localAddress.String()
}

// decodeInputs decodes the transaction method inputs
func (e eventData) decodeInputs(input []byte) (interface{}, error) {
	return e.methodABI.Inputs.Decode(
		input[len(e.getMethodID()):],
	)
}

// encodeInputs encodes the transaction method inputs
func (e eventData) encodeInputs(
	inputMap map[string]interface{},
) ([]byte, error) {
	return e.methodABI.Inputs.Encode(inputMap)
}
