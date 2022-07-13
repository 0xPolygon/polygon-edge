package ibft

import (
	"google.golang.org/protobuf/proto"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	protoIBFT "github.com/Trapesys/go-ibft/messages/proto"
)

func (i *Ibft) signMessage(msg *protoIBFT.Message) *protoIBFT.Message {
	raw, err := proto.Marshal(msg)
	if err != nil {
		panic("signMessage: cannot marshal")
	}

	sig, err := crypto.Sign(i.validatorKey, crypto.Keccak256(raw))
	if err != nil {
		panic("signMessage: cannot sign")
	}

	msg.Signature = sig

	return msg
}

func (i *Ibft) BuildPrePrepareMessage(proposal []byte, view *protoIBFT.View) *protoIBFT.Message {
	msg := &protoIBFT.Message{
		View: view,
		From: i.ID(),
		Type: protoIBFT.MessageType_PREPREPARE,
		Payload: &protoIBFT.Message_PreprepareData{PreprepareData: &protoIBFT.PrePrepareMessage{
			Proposal: proposal,
		}},
	}

	return i.signMessage(msg)
}

func (i *Ibft) BuildPrepareMessage(proposal []byte, view *protoIBFT.View) *protoIBFT.Message {
	block := &types.Block{}
	if err := block.UnmarshalRLP(proposal); err != nil {
		panic("BuildPrepareMessage: cannot unmarshal block")
		return nil
	}

	proposalHash := block.Header.Hash.Bytes()

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

func (i *Ibft) BuildCommitMessage(proposal []byte, view *protoIBFT.View) *protoIBFT.Message {
	block := &types.Block{}
	if err := block.UnmarshalRLP(proposal); err != nil {
		panic("BuildCommitMessage: cannot unmarshal block")
		//	log err
		return nil
	}

	proposalHash := block.Header.Hash.Bytes()

	seal, err := i.generateCommittedSeal(block.Header)
	if err != nil {
		//	log err
		panic("BuildCommitMessage: cannot generate seal")
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

func (i *Ibft) BuildRoundChangeMessage(height, round uint64) *protoIBFT.Message {
	msg := &protoIBFT.Message{
		View:    &protoIBFT.View{Height: height, Round: round},
		From:    i.ID(),
		Type:    protoIBFT.MessageType_ROUND_CHANGE,
		Payload: nil,
	}

	return i.signMessage(msg)
}

func (i *Ibft) generateCommittedSeal(header *types.Header) ([]byte, error) {
	//	TODO: just grab the hash ?
	hash, err := calculateHeaderHash(header)
	if err != nil {
		return nil, err
	}

	commitHash := crypto.Keccak256(hash, []byte{byte(protoIBFT.MessageType_COMMIT)})

	seal, err := crypto.Sign(i.validatorKey, crypto.Keccak256(commitHash))
	if err != nil {
		return nil, err
	}

	return seal, nil
}
