package polybft

import (
	"fmt"

	protoIBFT "github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
	"google.golang.org/protobuf/proto"
)

// Implementation of core.MessageConstructor interface

func signMessage(msg *protoIBFT.Message, key *wallet.Key) (*protoIBFT.Message, error) {
	raw, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal message:%w", err)
	}

	// TODO check message signature
	if msg.Signature, err = key.Sign(raw); err != nil {
		return nil, fmt.Errorf("cannot create message signature:%w", err)
	}

	return msg, nil
}

func (cr *consensusRuntime) BuildPrePrepareMessage(
	proposal []byte,
	certificate *protoIBFT.RoundChangeCertificate,
	view *protoIBFT.View,
) *protoIBFT.Message {
	block := types.Block{}
	if err := block.UnmarshalRLP(proposal); err != nil {
		cr.logger.Error(fmt.Sprintf("cannot unmarshal RLP:%s", err))

		return nil
	}

	proposalHash := block.Hash().Bytes()

	msg := protoIBFT.Message{
		View: view,
		From: cr.ID(),
		Type: protoIBFT.MessageType_PREPREPARE,
		Payload: &protoIBFT.Message_PreprepareData{
			PreprepareData: &protoIBFT.PrePrepareMessage{
				Proposal:     proposal,
				ProposalHash: proposalHash,
				Certificate:  certificate,
			},
		},
	}

	message, err := signMessage(&msg, cr.config.Key)
	if err != nil {
		cr.logger.Error("Cannot sign message", "Error", err)

		return nil
	}

	return message
}

func (cr *consensusRuntime) BuildPrepareMessage(proposalHash []byte, view *protoIBFT.View) *protoIBFT.Message {
	msg := protoIBFT.Message{
		View: view,
		From: cr.ID(),
		Type: protoIBFT.MessageType_PREPARE,
		Payload: &protoIBFT.Message_PrepareData{
			PrepareData: &protoIBFT.PrepareMessage{
				ProposalHash: proposalHash,
			},
		},
	}

	message, err := signMessage(&msg, cr.config.Key)
	if err != nil {
		cr.logger.Error("Cannot sign message.", "Error", err)

		return nil
	}

	return message
}

func (cr *consensusRuntime) BuildCommitMessage(proposalHash []byte, view *protoIBFT.View) *protoIBFT.Message {
	// TODO check committedSeal signature
	committedSeal, err := cr.config.Key.Sign(proposalHash) // .CreateCommittedSeal(proposalHash)
	if err != nil {
		cr.logger.Error("Cannot create committed seal message.", "Error", err)

		return nil
	}

	msg := protoIBFT.Message{
		View: view,
		From: cr.ID(),
		Type: protoIBFT.MessageType_COMMIT,
		Payload: &protoIBFT.Message_CommitData{
			CommitData: &protoIBFT.CommitMessage{
				ProposalHash:  proposalHash,
				CommittedSeal: committedSeal,
			},
		},
	}

	message, err := signMessage(&msg, cr.config.Key)
	if err != nil {
		cr.logger.Error("Cannot sign message", "Error", err)

		return nil
	}

	return message
}

func (cr *consensusRuntime) BuildRoundChangeMessage(
	proposal []byte,
	certificate *protoIBFT.PreparedCertificate,
	view *protoIBFT.View,
) *protoIBFT.Message {
	msg := protoIBFT.Message{
		View: view,
		From: cr.ID(),
		Type: protoIBFT.MessageType_ROUND_CHANGE,
		Payload: &protoIBFT.Message_RoundChangeData{RoundChangeData: &protoIBFT.RoundChangeMessage{
			LastPreparedProposedBlock: proposal,
			LatestPreparedCertificate: certificate,
		}},
	}

	signedMsg, err := signMessage(&msg, cr.config.Key)
	if err != nil {
		cr.logger.Error("Cannot sign message", "Error", err)

		return nil
	}

	return signedMsg
}
