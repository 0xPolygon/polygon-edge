package ibft

import (
	"github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/libp2p/go-libp2p/core/peer"
	protofrost "github.com/topos-protocol/go-topos-sequencer-client/frostclient/proto"
)

type transport interface {
	Multicast(msg *proto.Message) error
	MulticastFrost(msg *protofrost.FrostMessage) error
}

type gossipTransport struct {
	topic      *network.Topic
	topicFrost *network.Topic
}

func (g *gossipTransport) Multicast(msg *proto.Message) error {
	return g.topic.Publish(msg)
}

func (g *gossipTransport) MulticastFrost(msg *protofrost.FrostMessage) error {
	return g.topicFrost.Publish(msg)
}

func (i *backendIBFT) Multicast(msg *proto.Message) {
	if err := i.transport.Multicast(msg); err != nil {
		i.logger.Error("fail to gossip", "err", err)
	}
}

func (i *backendIBFT) MulticastFrost(msg *protofrost.FrostMessage) {
	if err := i.transport.MulticastFrost(msg); err != nil {
		i.logger.Error("fail to gossip frost message", "err", err)
	}
}

// setupTransport sets up the gossip transport protocol
func (i *backendIBFT) setupTransport() error {
	// Define a new topic
	topic, err := i.network.NewTopic(ibftProto, &proto.Message{})
	if err != nil {
		return err
	}

	// Subscribe to the newly created topic
	if err := topic.Subscribe(
		func(obj interface{}, _ peer.ID) {
			if !i.isActiveValidator() {
				return
			}

			msg, ok := obj.(*proto.Message)
			if !ok {
				i.logger.Error("invalid type assertion for message request")

				return
			}

			i.consensus.AddMessage(msg)

			i.logger.Debug(
				"validator message received",
				"type", msg.Type.String(),
				"height", msg.GetView().Height,
				"round", msg.GetView().Round,
				"addr", types.BytesToAddress(msg.From).String(),
			)
		},
	); err != nil {
		return err
	}

	// Define a new topic for frost
	topicFrost, err := i.network.NewTopic(frostProto, &protofrost.FrostMessage{})
	if err != nil {
		return err
	}

	// Subscribe to the frost Topic
	if err := topicFrost.Subscribe(
		func(obj interface{}, _ peer.ID) {
			if !i.isActiveValidator() {
				return
			}

			msg, ok := obj.(*protofrost.FrostMessage)
			if !ok {
				i.logger.Error("invalid type assertion for message request")

				return
			}
			i.logger.Info("Frost message gossiped to this node:", msg)

			err := i.frostBackend.ProcessGossipedMessages(msg)
			if err != nil {
				i.logger.Error("Failed to process gossiped message:", err)

				return
			}

			i.logger.Debug(
				"frost message received from validator network",
				"message id: ", msg.MessageId,
				" from: ", msg.From,
				" data: ", msg.Data,
			)
		},
	); err != nil {
		return err
	}

	i.transport = &gossipTransport{topic: topic, topicFrost: topicFrost}

	return nil
}
