package ibft

import (
	"google.golang.org/protobuf/proto"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	protoIBFT "github.com/Trapesys/go-ibft/messages/proto"
)

func (i *backendIBFT) signMessage(msg *protoIBFT.Message) *protoIBFT.Message {
	raw, err := proto.Marshal(msg)
	if err != nil {
		return nil
	}

	sig, err := crypto.Sign(i.validatorKey, crypto.Keccak256(raw))
	if err != nil {
		return nil
	}

	msg.Signature = sig

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
		Payload: &protoIBFT.Message_PreprepareData{PreprepareData: &protoIBFT.PrePrepareMessage{
			Proposal:     proposal,
			ProposalHash: proposalHash,
			Certificate:  certificate,
		}},
	}

	return i.signMessage(msg)
}

func (i *backendIBFT) BuildPrepareMessage(proposalHash []byte, view *protoIBFT.View) *protoIBFT.Message {
	msg := &protoIBFT.Message{
		View: view,
		From: i.ID(),
		Type: protoIBFT.MessageType_PREPARE,
		Payload: &protoIBFT.Message_PrepareData{PrepareData: &protoIBFT.PrepareMessage{
			ProposalHash: proposalHash,
		}},
	}

	return i.signMessage(msg)
}

func (i *backendIBFT) BuildCommitMessage(proposalHash []byte, view *protoIBFT.View) *protoIBFT.Message {
	seal, err := i.generateCommittedSeal(proposalHash)
	if err != nil {
		return nil
	}

	msg := &protoIBFT.Message{
		View: view,
		From: i.ID(),
		Type: protoIBFT.MessageType_COMMIT,
		Payload: &protoIBFT.Message_CommitData{CommitData: &protoIBFT.CommitMessage{
			ProposalHash:  proposalHash,
			CommittedSeal: seal,
		}},
	}

	return i.signMessage(msg)
}

func (i *backendIBFT) BuildRoundChangeMessage(height, round uint64) *protoIBFT.Message {
	msg := &protoIBFT.Message{
		View:    &protoIBFT.View{Height: height, Round: round},
		From:    i.ID(),
		Type:    protoIBFT.MessageType_ROUND_CHANGE,
		Payload: nil,
	}

	return i.signMessage(msg)
}

func (i *backendIBFT) generateCommittedSeal(proposalHash []byte) ([]byte, error) {
	commitHash := crypto.Keccak256(
		proposalHash,
		[]byte{byte(protoIBFT.MessageType_COMMIT)},
	)

	seal, err := crypto.Sign(i.validatorKey, crypto.Keccak256(commitHash))
	if err != nil {
		return nil, err
	}

	return seal, nil
}
