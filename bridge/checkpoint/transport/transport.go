package transport

import (
	"github.com/0xPolygon/polygon-edge/bridge/checkpoint/transport/proto"
	ctypes "github.com/0xPolygon/polygon-edge/bridge/checkpoint/types"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/hashicorp/go-hclog"
)

var transportProto = "/bridge/checkpoint/0.1"

type CheckpointTransport interface {
	Start() error
	SendCheckpoint(*CheckpointMessage) error
	SendAck(*AckMessage) error
	SendNoAck(*NoAckMessage) error
	Subscribe(func(interface{})) error
}

type libp2pGossipTransport struct {
	logger  hclog.Logger
	network *network.Server
	topic   *network.Topic
}

func NewLibp2pGossipTransport(logger hclog.Logger, network *network.Server) CheckpointTransport {
	return &libp2pGossipTransport{
		logger:  logger,
		network: network,
	}
}

func (t *libp2pGossipTransport) Start() error {
	topic, err := t.network.NewTopic(transportProto, &proto.SignedMessage{})
	if err != nil {
		return err
	}

	t.topic = topic

	return nil
}

func (t *libp2pGossipTransport) SendCheckpoint(proposal *CheckpointMessage) error {
	return t.topic.Publish(&proto.SignedMessage{
		Type:      proto.SignedMessage_CHECKPOINT,
		Payload:   proposal.Checkpoint.MarshalRLP(),
		Signature: proposal.Signature,
	})
}

func (t *libp2pGossipTransport) SendAck(ack *AckMessage) error {
	// return t.topic.Publish(...)
	return nil
}

func (t *libp2pGossipTransport) SendNoAck(noAck *NoAckMessage) error {
	// return t.topic.Publish(...)
	return nil
}

func (t *libp2pGossipTransport) Subscribe(handler func(interface{})) error {
	return t.topic.Subscribe(func(obj interface{}) {
		// TODO: convert proto.Message to CheckpointMessage,
		// AckMessage, or NoAckMessage defined in bridge/checkpoint/transport/types.go

		// and call handler

		protoMsg, ok := obj.(*proto.SignedMessage)
		if !ok {
			t.logger.Warn("received unexpected typed message", "message", obj)

			return
		}

		//	convert message to appropriate type
		var message interface{}

		switch protoMsg.Type {
		case proto.SignedMessage_CHECKPOINT:
			checkpoint := ctypes.Checkpoint{}
			if err := checkpoint.UnmarshalRLP(protoMsg.Payload); err != nil {
				t.logger.Error("unable to unmarshal payload from message", "err", err)

				return
			}

			message = &CheckpointMessage{
				Checkpoint: checkpoint,
				Signature:  protoMsg.Signature,
			}

		case proto.SignedMessage_ACK:
			//	TODO: phase2
		case proto.SignedMessage_NOACK:
			//	TODO: phase2
		}

		handler(message)
	})
}
