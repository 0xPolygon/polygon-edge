package ibft

import (
	"google.golang.org/protobuf/proto"

	protoIBFT "github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/0xPolygon/polygon-edge/types"
)

func (i *backendIBFT) signMessage(msg *protoIBFT.Message) *protoIBFT.Message {
	raw, err := proto.Marshal(msg)
	if err != nil {
		return nil
	}

	if msg.Signature, err = i.currentSigner.SignIBFTMessage(raw); err != nil {
		return nil
	}

	return msg
}

func (i *backendIBFT) BuildPrePrepareMessage(
	proposal []byte,
	certificate *protoIBFT.RoundChangeCertificate,
	view *protoIBFT.View,
) *protoIBFT.Message {
	block := &types.Block{}
	if err := block.UnmarshalRLP(proposal); err != nil {
		return nil
	}

	proposalHash := block.Hash().Bytes()

	msg := &protoIBFT.Message{
		View: view,
		From: i.ID(),
		Type: protoIBFT.MessageType_PREPREPARE,
		Payload: &protoIBFT.Message_PreprepareData{
			PreprepareData: &protoIBFT.PrePrepareMessage{
				Proposal:     proposal,
				ProposalHash: proposalHash,
				Certificate:  certificate,
			},
		},
	}

	return i.signMessage(msg)
}

func (i *backendIBFT) BuildPrepareMessage(proposalHash []byte, view *protoIBFT.View) *protoIBFT.Message {
	msg := &protoIBFT.Message{
		View: view,
		From: i.ID(),
		Type: protoIBFT.MessageType_PREPARE,
		Payload: &protoIBFT.Message_PrepareData{
			PrepareData: &protoIBFT.PrepareMessage{
				ProposalHash: proposalHash,
			},
		},
	}

	return i.signMessage(msg)
}

func (i *backendIBFT) BuildCommitMessage(proposalHash []byte, view *protoIBFT.View) *protoIBFT.Message {
	committedSeal, err := i.currentSigner.CreateCommittedSeal(proposalHash)
	if err != nil {
		i.logger.Error("Unable to build commit message, %v", err)

		return nil
	}

	msg := &protoIBFT.Message{
		View: view,
		From: i.ID(),
		Type: protoIBFT.MessageType_COMMIT,
		Payload: &protoIBFT.Message_CommitData{
			CommitData: &protoIBFT.CommitMessage{
				ProposalHash:  proposalHash,
				CommittedSeal: committedSeal,
			},
		},
	}

	return i.signMessage(msg)
}

func (i *backendIBFT) BuildRoundChangeMessage(
	proposal []byte,
	certificate *protoIBFT.PreparedCertificate,
	view *protoIBFT.View,
) *protoIBFT.Message {
	msg := &protoIBFT.Message{
		View: view,
		From: i.ID(),
		Type: protoIBFT.MessageType_ROUND_CHANGE,
		Payload: &protoIBFT.Message_RoundChangeData{RoundChangeData: &protoIBFT.RoundChangeMessage{
			LastPreparedProposedBlock: proposal,
			LatestPreparedCertificate: certificate,
		}},
	}

	return i.signMessage(msg)
}
