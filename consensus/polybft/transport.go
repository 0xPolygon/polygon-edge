package polybft

import (
	"encoding/json"
	"fmt"

	"github.com/0xPolygon/go-ibft/messages/proto"
	pbftproto "github.com/0xPolygon/polygon-edge/consensus/polybft/proto"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Bridge transport is an abstraction of network layer for a bridge
type BridgeTransport interface {
	Multicast(msg interface{})
}

type runtimeTransportWrapper struct {
	bridgeTopic *network.Topic
	logger      hclog.Logger
}

var _ BridgeTransport = (*runtimeTransportWrapper)(nil)

// Multicast publishes any message as pbftproto.TransportMessage
func (g *runtimeTransportWrapper) Multicast(msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		g.logger.Warn("failed to marshal bridge message", "err", err)

		return
	}

	err = g.bridgeTopic.Publish(&pbftproto.TransportMessage{Data: data})
	if err != nil {
		g.logger.Warn("failed to gossip bridge message", "err", err)
	}
}

// subscribeToBridgeTopic subscribes for bridge topic
func (cr *consensusRuntime) subscribeToBridgeTopic(topic *network.Topic) error {
	return topic.Subscribe(func(obj interface{}, _ peer.ID) {
		msg, ok := obj.(*pbftproto.TransportMessage)
		if !ok {
			cr.logger.Warn("failed to deliver message", "err", "invalid msg")

			return
		}

		var transportMsg *TransportMessage

		if err := json.Unmarshal(msg.Data, &transportMsg); err != nil {
			cr.logger.Warn("failed to deliver message", "err", err)

			return
		}

		if _, err := cr.deliverMessage(transportMsg); err != nil {
			cr.logger.Warn("failed to deliver message", "err", err)
		}
	})
}

// subscribeToIbftTopic subscribes to ibft topic
func (p *Polybft) subscribeToIbftTopic() error {
	return p.consensusTopic.Subscribe(func(obj interface{}, _ peer.ID) {
		// this check is from ibft impl
		if !p.runtime.isActiveValidator() {
			return
		}

		msg, ok := obj.(*proto.Message)
		if !ok {
			p.logger.Error("consensus engine: invalid type assertion for message request")

			return
		}

		p.ibft.AddMessage(msg)

		p.logger.Debug(
			"validator message received",
			"type", msg.Type.String(),
			"height", msg.GetView().Height,
			"round", msg.GetView().Round,
			"addr", types.BytesToAddress(msg.From).String(),
		)
	})
}

// createTopics create all topics for a PolyBft instance
func (p *Polybft) createTopics() (err error) {
	if p.consensusConfig.IsBridgeEnabled() {
		// create bridge topic
		p.bridgeTopic, err = p.config.Network.NewTopic(bridgeProto, &pbftproto.TransportMessage{})
		if err != nil {
			return fmt.Errorf("failed to create bridge topic. Error: %w", err)
		}
	}

	// create pbft topic
	p.consensusTopic, err = p.config.Network.NewTopic(pbftProto, &proto.Message{})
	if err != nil {
		return fmt.Errorf("failed to create pbft topic. Error: %w", err)
	}

	return nil
}

// Multicast is implementation of core.Transport interface
func (p *Polybft) Multicast(msg *proto.Message) {
	if err := p.consensusTopic.Publish(msg); err != nil {
		p.logger.Warn("failed to multicast consensus message", "err", err)
	}
}
