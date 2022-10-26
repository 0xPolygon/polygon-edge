package polybft

import (
	"encoding/json"
	"fmt"

	"github.com/0xPolygon/go-ibft/messages/proto"
	pbftproto "github.com/0xPolygon/polygon-edge/consensus/polybft/proto"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Transport is an abstraction of network layer for a bridge
type BridgeTransport interface {
	Gossip(msg interface{}) error
}

// Transport is an abstraction of network layer for a consensus
type ConsensusTransport interface {
	Multicast(msg *proto.Message) error
}

type runtimeTransportWrapper struct {
	bridgeTopic    *network.Topic
	consensusTopic *network.Topic
}

var (
	_ BridgeTransport    = (*runtimeTransportWrapper)(nil)
	_ ConsensusTransport = (*runtimeTransportWrapper)(nil)
)

func newRuntimeTransportWrapper(bridgeTopic, consensusTopic *network.Topic) *runtimeTransportWrapper {
	return &runtimeTransportWrapper{bridgeTopic: bridgeTopic, consensusTopic: consensusTopic}
}

func (g *runtimeTransportWrapper) Gossip(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	protoMsg := &pbftproto.TransportMessage{
		Data: data,
	}

	return g.bridgeTopic.Publish(protoMsg)
}

func (g *runtimeTransportWrapper) Multicast(msg *proto.Message) error {
	return g.consensusTopic.Publish(msg)
}

func (cr *consensusRuntime) Multicast(msg *proto.Message) {
	if err := cr.config.ConsensusTransport.Multicast(msg); err != nil {
		cr.logger.Error("fail to gossip", "err", err)
	}
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

// subscribeToBridgeTopic subscribes for bridge topic
func (p *Polybft) subscribeToBridgeTopic() error {
	return p.consensusTopic.Subscribe(func(obj interface{}, _ peer.ID) {
		msg, ok := obj.(*pbftproto.TransportMessage)
		if !ok {
			p.logger.Warn("failed to deliver message", "err", "invalid msg")

			return
		}

		var transportMsg *TransportMessage

		if err := json.Unmarshal(msg.Data, &transportMsg); err != nil {
			p.logger.Warn("failed to deliver message", "err", err)

			return
		}

		if _, err := p.runtime.deliverMessage(transportMsg); err != nil {
			p.logger.Warn("failed to deliver message", "err", err)
		}
	})
}

// subscribeToTopics subscribes to all topics
func (p *Polybft) subscribeToTopics() (err error) {
	if p.consensusConfig.IsBridgeEnabled() {
		if err = p.subscribeToBridgeTopic(); err != nil {
			return err
		}
	}

	err = p.consensusTopic.Subscribe(func(obj interface{}, from peer.ID) {
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

	if err != nil {
		return fmt.Errorf("topic subscription failed: %w", err)
	}

	return nil
}
