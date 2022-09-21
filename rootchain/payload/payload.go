package payload

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/rootchain"
	"github.com/0xPolygon/polygon-edge/rootchain/proto"
	googleProto "google.golang.org/protobuf/proto"
)

var (
	ErrUnknownPayloadType = errors.New("unknown payload type")
)

// GetEventPayload retrieves a concrete payload implementation
// based on the passed in byte array and payload type
func GetEventPayload(
	eventPayload []byte,
	payloadType uint64,
) (rootchain.Payload, error) {
	switch rootchain.PayloadType(payloadType) {
	case rootchain.ValidatorSetPayloadType:
		// Unmarshal the data
		vsProto := &proto.ValidatorSetPayload{}
		if err := googleProto.Unmarshal(eventPayload, vsProto); err != nil {
			return nil, fmt.Errorf("unable to unmarshal proto payload, %w", err)
		}

		setInfo := make([]ValidatorSetInfo, len(vsProto.ValidatorsInfo))

		// Extract the specific info
		for index, info := range vsProto.ValidatorsInfo {
			setInfo[index] = ValidatorSetInfo{
				Address:      info.Address,
				BLSPublicKey: info.BlsPubKey,
			}
		}

		// Return the specific Payload implementation
		return NewValidatorSetPayload(setInfo), nil
	default:
		return nil, ErrUnknownPayloadType
	}
}
