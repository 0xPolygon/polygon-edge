package transport

import (
	"github.com/0xPolygon/polygon-edge/bridge/sam"
	"github.com/0xPolygon/polygon-edge/bridge/transport/proto"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/hashicorp/go-hclog"
)

var transportProto = "/bridge/sam/0.1"

type MessageTransport interface {
	Start() error
	Publish(*sam.Message, []byte) error
	Subscribe(func(*sam.Message, []byte)) error
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

func (t *libp2pGossipTransport) Publish(message *sam.Message, signature []byte) error {
	return t.topic.Publish(&proto.SignedMessage{
		Id:        message.ID,
		Body:      message.Body,
		Signature: signature,
	})
}

func (t *libp2pGossipTransport) Subscribe(handler func(*sam.Message, []byte)) error {
	return t.topic.Subscribe(func(obj interface{}) {
		protoMessage, ok := obj.(*proto.SignedMessage)
		if !ok {
			t.logger.Warn("received unexpected typed message", "message", obj)

			return
		}

		message := toSAMMessage(protoMessage)
		signature := protoMessage.Signature

		handler(message, signature)
	})
}

func toSAMMessage(protoMessage *proto.SignedMessage) *sam.Message {
	return &sam.Message{
		ID:   protoMessage.Id,
		Body: protoMessage.Body,
	}
}
