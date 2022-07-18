package ibft

import (
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/Trapesys/go-ibft/messages/proto"
	"github.com/libp2p/go-libp2p-core/peer"
)

type transport interface {
	Multicast(msg *proto.Message) error
}

type gossipTransport struct {
	topic *network.Topic
}

// Gossip publishes a new message to the topic
func (g *gossipTransport) Multicast(msg *proto.Message) error {
	return g.topic.Publish(msg)
}

func (i *Ibft) Multicast(msg *proto.Message) {
	if err := i.transport.Multicast(msg); err != nil {
		i.logger.Error("fail to gossip", "err", err)
	}
}

// setupTransport sets up the gossip transport protocol
func (i *Ibft) setupTransport() error {
	// Define a new topic
	topic, err := i.network.NewTopic(ibftProto, &proto.Message{})
	if err != nil {
		return err
	}

	// Subscribe to the newly created topic
	err = topic.Subscribe(func(obj interface{}, _ peer.ID) {
		msg, ok := obj.(*proto.Message)
		if !ok {
			i.logger.Error("invalid type assertion for message request")

			return
		}

		if !i.isSealing() {
			// if we are not sealing we do not care about the messages
			// but we need to subscribe to propagate the messages
			return
		}

		i.consensus.AddMessage(msg)

		i.logger.Info(
			"validator message received",
			"type", msg.Type.String(),
			"height", msg.GetView().Height,
			"round", msg.GetView().Round,
			"addr", types.BytesToAddress(msg.From).String(),
		)
	})

	if err != nil {
		return err
	}

	i.transport = &gossipTransport{topic: topic}

	return nil
}
