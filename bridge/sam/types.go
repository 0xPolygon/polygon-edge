package sam

import "github.com/0xPolygon/polygon-edge/types"

type Message struct {
	Hash types.Hash
	Data interface{}
}

type MessageSignature struct {
	Hash      types.Hash
	Address   types.Address
	Signature []byte
}

type ReadyMessage struct {
	Data       interface{}
	Hash       types.Hash
	Signatures [][]byte
}

type Signer interface {
	Sign([]byte) ([]byte, error)
	Address() types.Address
	RecoverAddress(digest, signature []byte) (types.Address, error)
}

type Pool interface {
	AddMessage(*Message)
	AddSignature(*MessageSignature)
	ConsumeMessage(types.Hash)
	GetReadyMessages() []ReadyMessage
	UpdateValidatorSet([]types.Address, uint64)
	IsMessageKnown(hash types.Hash) bool
	GetSignatureCount(hash types.Hash) uint64
}
