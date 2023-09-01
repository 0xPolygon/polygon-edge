package polybft

import (
	"crypto/rand"
	"fmt"

	ibftProto "github.com/0xPolygon/go-ibft/messages/proto"
	polybftProto "github.com/0xPolygon/polygon-edge/consensus/polybft/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
)

// BridgeTransport is an abstraction of network layer for a bridge
type BridgeTransport interface {
	Multicast(msg interface{})
}

// subscribeToIbftTopic subscribes to ibft topic
func (p *Polybft) subscribeToIbftTopic() error {
	return p.consensusTopic.Subscribe(func(obj interface{}, _ peer.ID) {
		if !p.runtime.IsActiveValidator() {
			return
		}

		msg, ok := obj.(*ibftProto.Message)
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
	if msg.Type == ibftProto.MessageType_COMMIT {
		sender := types.BytesToAddress(msg.From)
		localAddr := types.Address(p.key.Address())

		if sender == localAddr {
			tamperedMsg, ok := proto.Clone(msg).(*ibftProto.Message)

			if ok {
				p.logger.Debug("wrong type assertion")
			}

			tamperedMsg.GetCommitData().ProposalHash = generateRandomHash()
			tamperedMsg.Signature = nil

			tamperedMsg, err := p.key.SignIBFTMessage(tamperedMsg)

			if err != nil {
				p.logger.Warn("failed to sign message", "error", err)
			}

			if err = p.consensusTopic.Publish(tamperedMsg); err != nil {
				p.logger.Warn("failed to multicast second consensus message", "error", err)
			}
		}
	}

	if err := p.consensusTopic.Publish(msg); err != nil {
		p.logger.Warn("failed to multicast consensus message", "error", err)
	}
}

func generateRandomHash() []byte {
	result := make([]byte, types.HashLength)
	_, _ = rand.Reader.Read(result)

	return result
}
