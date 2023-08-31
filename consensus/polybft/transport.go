package polybft

import (
	"crypto/rand"
	"fmt"

	ibftProto "github.com/0xPolygon/go-ibft/messages/proto"
	polybftProto "github.com/0xPolygon/polygon-edge/consensus/polybft/proto"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"
	protobuf "google.golang.org/protobuf/proto"
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
	p.logger.Error(fmt.Sprintf("[MulticastPublish]"))

	if msg.Type == ibftProto.MessageType_COMMIT {
		sender := types.BytesToAddress(msg.From)
		localAddr := types.Address(p.key.Address())
		p.logger.Debug("[Multicast]", "msg sender", sender.String(), "local node address", localAddr.String())

		if sender == localAddr {
			tamperedMsg := proto.Clone(msg).(*ibftProto.Message)
			tamperedMsg.GetCommitData().ProposalHash = generateRandomHash()
			tamperedMsg.Signature = nil

			tamperedMsgRaw, _ := protobuf.Marshal(tamperedMsg)

			p.logger.Error("MsgRaw", "before", tamperedMsgRaw, "cryptokeccakBefore:", crypto.Keccak256(tamperedMsgRaw))

			tamperedMsg, err := p.key.SignIBFTMessage(tamperedMsg)

			if err != nil {
				p.logger.Error("Error while signing", "error", err)
			}

			p.logger.Error(fmt.Sprintf("[Polybft]Multicast(publish1): %+v", tamperedMsg))
			msgNoSigTampered, err := tamperedMsg.PayloadNoSig()

			if err != nil {
				p.logger.Error("NoPayload")
			}

			pub, _ := crypto.RecoverPubkey(tamperedMsg.Signature, crypto.Keccak256(msgNoSigTampered))

			p.logger.Error("msgRawAfter", "after", msgNoSigTampered, "CryptoKeccakAfter", crypto.Keccak256(msgNoSigTampered))
			p.logger.Error("PubKey", "after", pub)
			signerAddress, err := RecoverAddressFromSignature(tamperedMsg.Signature, msgNoSigTampered)

			if err != nil {
				p.logger.Error("failed to recover address from signature: %w", err)
			}

			p.logger.Error("Signer", "Address", signerAddress.String())

			err = p.consensusTopic.Publish(tamperedMsg)
			if err != nil {
				p.logger.Error("Error while sending byzantian message", "error", err)
			}
		}
	}

	p.logger.Error("[Polybft]Multicast(publish2): %+v", msg)
	if err := p.consensusTopic.Publish(msg); err != nil {
		p.logger.Warn("failed to multicast consensus message", "error", err)
	}
}
func RecoverAddressFromSignature(sig, rawContent []byte) (types.Address, error) {
	pub, err := crypto.RecoverPubkey(sig, crypto.Keccak256(rawContent))
	if err != nil {
		return types.Address{}, fmt.Errorf("cannot recover address from signature: %w", err)
	}

	return crypto.PubKeyToAddress(pub), nil
}

func generateRandomHash() []byte {
	result := make([]byte, types.HashLength)
	_, _ = rand.Reader.Read(result)

	return result
}
