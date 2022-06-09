package signer

import (
	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/validators"
	"github.com/0xPolygon/polygon-edge/types"
)

// KeyManager is a delegated object to sign data
type KeyManager interface {
	Address() types.Address
	NewEmptyIstanbulExtra() *IstanbulExtra
	NewEmptyCommittedSeal() Sealer
	SignSeal([]byte) ([]byte, error)
	SignCommittedSeal([]byte) ([]byte, error)
	Ecrecover(sig []byte, digest []byte) (types.Address, error)
	GenerateCommittedSeals(map[types.Address][]byte, *IstanbulExtra) (Sealer, error)
	VerifyCommittedSeal(Sealer, []byte, validators.ValidatorSet) (int, error)
	SignIBFTMessage(msg *proto.MessageReq) error
	ValidateIBFTMessage(msg *proto.MessageReq) error
}
