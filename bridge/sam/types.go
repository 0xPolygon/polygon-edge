package sam

import "github.com/0xPolygon/polygon-edge/types"

type Message struct {
	ID   uint64
	Body []byte
}

type SignedMessage struct {
	Message
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
}

type SignatureRecoverer interface {
	Recover([]byte) (types.Address, error)
}

type Pool interface {
	Add(*SignedMessage)
	MarkAsKnown(uint64)
	Consume(uint64)
	GetReadyMessages() []MessageAndSignatures
	UpdateValidatorSet([]types.Address, uint64)
}

type Manager interface {
	Start() error
	Close() error
	AddMessage(message *Message) error
	GetReadyMessages() []MessageAndSignatures
	UpdateValidatorSet([]types.Address, uint64)
}
