package polybft

import (
	"fmt"

	ibftProto "github.com/0xPolygon/go-ibft/messages/proto"
	polybftProto "github.com/0xPolygon/polygon-edge/consensus/polybft/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

// BridgeTransport is an abstraction of network layer for a bridge
type BridgeTransport interface {
	Multicast(msg interface{})
}

// subscribeToIbftTopic subscribes to ibft topic
func (p *Polybft) subscribeToIbftTopic() error {
	return p.consensusTopic.Subscribe(func(payload interface{}, _ peer.ID) {
		if !p.runtime.IsActiveValidator() {
			return
		}

		msg, ok := payload.(*ibftProto.Message)
		if !ok {
			p.logger.Error("consensus engine: invalid type assertion for message request")

			return
		}

		for _, handler := range p.ibftMsgHandlers {
			handler := handler

			go func() {
				select {
				case <-p.closeCh:
					return
				default:
					handler.Handle(msg)
				}
			}()
		}

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
	if p.genesisClientConfig.IsBridgeEnabled() {
		// this is ok to ask, since bridge configuration will not be changed through governance
		p.bridgeTopic, err = p.config.Network.NewTopic(bridgeProto, &polybftProto.TransportMessage{})
		if err != nil {
			return fmt.Errorf("failed to create bridge topic: %w", err)
		}
	}

	p.consensusTopic, err = p.config.Network.NewTopic(pbftProto, &ibftProto.Message{})
	if err != nil {
		return fmt.Errorf("failed to create consensus topic: %w", err)
	}

	return nil
}

// Multicast is implementation of core.Transport interface
func (p *Polybft) Multicast(msg *ibftProto.Message) {
	if err := p.consensusTopic.Publish(msg); err != nil {
		p.logger.Warn("failed to multicast consensus message", "error", err)
	}
}
