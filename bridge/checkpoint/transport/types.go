package transport

import (
	ctypes "github.com/0xPolygon/polygon-edge/bridge/checkpoint/types"
	"github.com/0xPolygon/polygon-edge/types"
)

type MsgType string

const (
	MsgCheckpoint MsgType = "checkpoint"
	MsgAck        MsgType = "ack"
	MsgNoAck      MsgType = "noack"
)

type Message interface {
	Type() MsgType
	Hash() types.Hash
	Sig() []byte
}

type CheckpointMessage struct {
	Checkpoint ctypes.Checkpoint
	Signature  []byte
}

func (c *CheckpointMessage) Type() MsgType {
	return MsgCheckpoint
}

func (c *CheckpointMessage) Hash() types.Hash {
	return c.Checkpoint.Hash()
}

func (c *CheckpointMessage) Sig() []byte {
	return c.Signature
}

type AckMessage struct {
	Ack       ctypes.Ack
	Signature []byte
}

func (c *AckMessage) Type() MsgType {
	return MsgAck
}

func (c *AckMessage) Hash() types.Hash {
	return c.Ack.Hash()
}

func (c *AckMessage) Sig() []byte {
	return c.Signature
}

type NoAckMessage struct {
	NoAck     ctypes.NoAck
	Signature []byte
}

func (c *NoAckMessage) Type() MsgType {
	return MsgNoAck
}

func (c *NoAckMessage) Hash() types.Hash {
	return c.NoAck.Hash()
}

func (c *NoAckMessage) Sig() []byte {
	return c.Signature
}
