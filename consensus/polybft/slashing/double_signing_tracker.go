package slashing

import (
	ibftProto "github.com/0xPolygon/go-ibft/messages/proto"

	"github.com/0xPolygon/polygon-edge/types"
)

type SenderMessagesMap map[types.Address][]*ibftProto.CommitMessage

type DoubleSignEvidence struct {
	signer   types.Address
	round    uint64
	messages []*ibftProto.CommitMessage
}

func newDoubleSignEvidence(signer types.Address, round uint64,
	messages []*ibftProto.CommitMessage) *DoubleSignEvidence {
	return &DoubleSignEvidence{signer: signer, round: round, messages: messages}
}

type MessagesMap map[uint64]map[uint64]SenderMessagesMap

// getSenderMsgs returns commit messages for given height, round and sender
func (m MessagesMap) getSenderMsgs(view *ibftProto.View, sender types.Address) []*ibftProto.CommitMessage {
	if roundMsgs, ok := m[view.Height]; ok {
		if senderMsgsMap, ok := roundMsgs[view.Round]; ok {
			return senderMsgsMap[sender]
		}
	}

	return nil
}

// registerSenderMsgs registers messages for the given height, round and sender
func (m MessagesMap) registerSenderMsgs(view *ibftProto.View, sender types.Address, msgs []*ibftProto.CommitMessage) {
	var senderMsgs SenderMessagesMap

	if roundMsgs, ok := m[view.Height]; !ok {
		roundMsgs = make(map[uint64]SenderMessagesMap)
		m[view.Height] = roundMsgs
		senderMsgs = createSenderMsgsMap(view.Round, roundMsgs)
	} else if senderMsgs, ok = roundMsgs[view.Round]; !ok {
		senderMsgs = createSenderMsgsMap(view.Round, roundMsgs)
	}

	senderMsgs[sender] = msgs
}

// createSenderMsgsMap initializes senders message map for the given round
func createSenderMsgsMap(round uint64, roundMsgs map[uint64]SenderMessagesMap) SenderMessagesMap {
	sendersMsgs := make(SenderMessagesMap)
	roundMsgs[round] = sendersMsgs

	return sendersMsgs
}

type doubleSigningTracker struct {
	commit MessagesMap
}

func NewDoubleSigningTracker() *doubleSigningTracker {
	return &doubleSigningTracker{
		commit: make(MessagesMap),
	}
}

// Handle is implementation of IBFTMessageHandler interface, which handles IBFT consensus messages
func (t *doubleSigningTracker) Handle(msg *ibftProto.Message) {
	// track only commit messages
	commitMsg := msg.GetCommitData()
	if commitMsg == nil {
		return
	}

	sender := types.BytesToAddress(msg.From)

	senderMsgs := t.commit.getSenderMsgs(msg.View, sender)
	if senderMsgs == nil {
		senderMsgs = []*ibftProto.CommitMessage{commitMsg}
	} else {
		senderMsgs = append(senderMsgs, commitMsg)
	}

	t.commit.registerSenderMsgs(msg.View, sender, senderMsgs)
}

func (t *doubleSigningTracker) PruneByHeight(height uint64) {
	// Delete all height maps up until the specified
	for msgHeight := range t.commit {
		if msgHeight < height {
			delete(t.commit, height)
		}
	}
}

// GetEvidences returns double signing evidences for the given height
func (t *doubleSigningTracker) GetEvidences(height uint64) []*DoubleSignEvidence {
	roundMsgs, ok := t.commit[height]
	if !ok {
		return nil
	}

	var (
		evidences = []*DoubleSignEvidence{}
		evidence  *DoubleSignEvidence
	)

	for round, senderMsgs := range roundMsgs {
		for address, msgs := range senderMsgs {
			if len(msgs) <= 1 {
				continue
			}

			firstProposalHash := types.BytesToHash(msgs[0].ProposalHash)
			for _, msg := range msgs[1:] {
				if firstProposalHash != types.BytesToHash(msg.ProposalHash) {
					if evidence == nil {
						evidence = newDoubleSignEvidence(address, round, []*ibftProto.CommitMessage{})
						evidences = append(evidences, evidence)
					}

					evidence.messages = append(evidence.messages, msg)
				}
			}
		}
	}

	return evidences
}
