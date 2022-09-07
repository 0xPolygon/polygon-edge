package transport

import (
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/rootchain/proto"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
)

const transportProto = "/rootchain/0.1"

type libp2pGossipTransport struct {
	logger  hclog.Logger
	network *network.Server
	topic   *network.Topic
}

func NewLibp2pGossipTransport(
	logger hclog.Logger,
	network *network.Server,
) *libp2pGossipTransport {
	return &libp2pGossipTransport{
		logger:  logger.Named("rootchain transport"),
		network: network,
	}
}

func (t *libp2pGossipTransport) Start() error {
	topic, err := t.network.NewTopic(transportProto, &proto.SAM{})
	if err != nil {
		return err
	}

	t.topic = topic

	return nil
}

func (t *libp2pGossipTransport) Publish(message *proto.SAM) error {
	return t.topic.Publish(message)
}

func (t *libp2pGossipTransport) Subscribe(handler func(*proto.SAM)) error {
	return t.topic.Subscribe(func(obj interface{}, _ peer.ID) {
		protoMessage, ok := obj.(*proto.SAM)
		if !ok {
			t.logger.Warn("received unexpected typed message", "message", obj)

			return
		}

		handler(protoMessage)
	})
}
