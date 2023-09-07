//go:build doubleSigner

package polybft

import (
	"crypto/rand"

	ibftProto "github.com/0xPolygon/go-ibft/messages/proto"
	"google.golang.org/protobuf/proto"

	"github.com/0xPolygon/polygon-edge/types"
)

func (p *Polybft) ibftMsgMulticast(msg *ibftProto.Message) {
	if msg.Type != ibftProto.MessageType_COMMIT {
		return
	}

	sender := types.BytesToAddress(msg.From)
	localAddr := types.Address(p.key.Address())

	if sender != localAddr {
		return
	}

	tamperedMsg, _ := proto.Clone(msg).(*ibftProto.Message)
	tamperedMsg.GetCommitData().ProposalHash = generateRandomHash()
	tamperedMsg.Signature = nil

	tamperedMsg, err := p.key.SignIBFTMessage(tamperedMsg)
	if err != nil {
		p.logger.Warn("failed to sign incorrect proposal hash message", "error", err)
	}

	if err = p.consensusTopic.Publish(tamperedMsg); err != nil {
		p.logger.Warn("failed to multicast incorrect proposal hash consensus message", "error", err)
	}
}

func generateRandomHash() []byte {
	result := make([]byte, types.HashLength)
	_, _ = rand.Reader.Read(result)

	return result
}
