package transport

import (
	"github.com/0xPolygon/polygon-edge/bridge/statesync/transport/proto"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

var transportProto = "/bridge/statesync/0.1"

type MessageTransport interface {
	Start() error
	Publish(*SignedMessage) error
	Subscribe(func(*SignedMessage)) error
}

type libp2pGossipTransport struct {
	logger  hclog.Logger
	network *network.Server
	topic   *network.Topic
}

func NewLibp2pGossipTransport(logger hclog.Logger, network *network.Server) MessageTransport {
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

func (t *libp2pGossipTransport) Publish(message *SignedMessage) error {
	return t.topic.Publish(&proto.SignedMessage{
		Hash:      message.Hash[:],
		Signature: message.Signature,
	})
}

func (t *libp2pGossipTransport) Subscribe(handler func(*SignedMessage)) error {
	return t.topic.Subscribe(func(obj interface{}) {
		protoMessage, ok := obj.(*proto.SignedMessage)
		if !ok {
			t.logger.Warn("received unexpected typed message", "message", obj)

			return
		}

		message := toSignedMessage(protoMessage)

		handler(message)
	})
}

func toSignedMessage(protoMessage *proto.SignedMessage) *SignedMessage {
	return &SignedMessage{
		Hash:      types.BytesToHash(protoMessage.Hash),
		Signature: protoMessage.Signature,
	}
}
