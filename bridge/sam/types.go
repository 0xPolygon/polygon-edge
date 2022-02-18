package sam

import "github.com/0xPolygon/polygon-edge/types"

type Message []byte

type SignedMessage struct {
	Message   Message
	Hash      types.Hash
	Address   types.Address
	Signature []byte
}

type MessageAndSignatures struct {
	Message    *Message
	Signatures [][]byte
}

type Signer interface {
	Sign([]byte) ([]byte, error)
	Address() types.Address
	RecoverAddress(digest, signature []byte) (types.Address, error)
}

type Pool interface {
	Add(*SignedMessage)
	MarkAsKnown(types.Hash)
	Consume(types.Hash)
	GetReadyMessages() []MessageAndSignatures
	UpdateValidatorSet([]types.Address, uint64)
}
