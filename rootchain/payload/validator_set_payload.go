package payload

import (
	"github.com/0xPolygon/polygon-edge/rootchain"
	rootProto "github.com/0xPolygon/polygon-edge/rootchain/proto"
	"google.golang.org/protobuf/proto"
)

// ValidatorSetPayload defines a payload that contains
// an array of tuples (validatorAddr, BLSPublicKey)
type ValidatorSetPayload struct {
	setInfo []ValidatorSetInfo
}

type ValidatorSetInfo struct {
	Address      []byte
	BLSPublicKey []byte
}

func NewValidatorSetPayload(setInfo []ValidatorSetInfo) *ValidatorSetPayload {
	return &ValidatorSetPayload{
		setInfo: setInfo,
	}
}

func (v *ValidatorSetPayload) Get() (rootchain.PayloadType, []byte) {
	payload, _ := v.Marshal()

	return rootchain.ValidatorSetPayloadType, payload
}

func (v *ValidatorSetPayload) GetSetInfo() []ValidatorSetInfo {
	return v.setInfo
}

func (v *ValidatorSetPayload) toProto() *rootProto.ValidatorSetPayload {
	validatorsInfo := make([]*rootProto.ValidatorInfoBLS, len(v.setInfo))

	for index, info := range v.setInfo {
		validatorsInfo[index] = &rootProto.ValidatorInfoBLS{
			Address:   info.Address,
			BlsPubKey: info.BLSPublicKey,
		}
	}

	return &rootProto.ValidatorSetPayload{
		ValidatorsInfo: validatorsInfo,
	}
}

// Marshal marshals the SAM into bytes
func (v *ValidatorSetPayload) Marshal() ([]byte, error) {
	return proto.Marshal(v.toProto())
}
