package slashing

import (
	"testing"

	ibftProto "github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/stretchr/testify/require"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
)

func buildPrePrepareMessage(t *testing.T, view *ibftProto.View,
	key *wallet.Key, proposalHash types.Hash) *ibftProto.Message {
	t.Helper()

	prePrepareMsg := &ibftProto.Message{
		View: view,
		From: key.Address().Bytes(),
		Type: ibftProto.MessageType_PREPREPARE,
		Payload: &ibftProto.Message_PreprepareData{
			PreprepareData: &ibftProto.PrePrepareMessage{
				Proposal: &ibftProto.Proposal{
					RawProposal: proposalHash.Bytes(),
					Round:       1,
				},
				ProposalHash: proposalHash.Bytes(),
				Certificate:  &ibftProto.RoundChangeCertificate{},
			},
		},
	}

	prePrepareMsg, err := key.SignIBFTMessage(prePrepareMsg)
	require.NoError(t, err)

	return prePrepareMsg
}

func buildPrepareMessage(t *testing.T, view *ibftProto.View,
	key *wallet.Key, proposalHash types.Hash) *ibftProto.Message {
	t.Helper()

	prepareMsg := &ibftProto.Message{
		View: view,
		From: key.Address().Bytes(),
		Type: ibftProto.MessageType_PREPARE,
		Payload: &ibftProto.Message_PrepareData{
			PrepareData: &ibftProto.PrepareMessage{
				ProposalHash: proposalHash.Bytes(),
			},
		},
	}

	prepareMsg, err := key.SignIBFTMessage(prepareMsg)
	require.NoError(t, err)

	return prepareMsg
}

func buildCommitMessage(t *testing.T, view *ibftProto.View,
	key *wallet.Key, proposalHash types.Hash) *ibftProto.Message {
	t.Helper()

	seal, err := key.Sign(proposalHash.Bytes())
	require.NoError(t, err)

	commitMsg := &ibftProto.Message{
		View: view,
		From: key.Address().Bytes(),
		Type: ibftProto.MessageType_COMMIT,
		Payload: &ibftProto.Message_CommitData{
			CommitData: &ibftProto.CommitMessage{
				ProposalHash:  proposalHash.Bytes(),
				CommittedSeal: seal,
			},
		},
	}

	commitMsg, err = key.SignIBFTMessage(commitMsg)
	require.NoError(t, err)

	return commitMsg
}

func buildRoundChangeMessage(t *testing.T, view *ibftProto.View, key *wallet.Key) *ibftProto.Message {
	t.Helper()

	roundChangeMsg := &ibftProto.Message{
		View: view,
		From: key.Address().Bytes(),
		Type: ibftProto.MessageType_ROUND_CHANGE,
		Payload: &ibftProto.Message_RoundChangeData{
			RoundChangeData: &ibftProto.RoundChangeMessage{
				LastPreparedProposal:      &ibftProto.Proposal{},
				LatestPreparedCertificate: &ibftProto.PreparedCertificate{},
			},
		},
	}

	roundChangeMsg, err := key.SignIBFTMessage(roundChangeMsg)
	require.NoError(t, err)

	return roundChangeMsg
}

func assertSenderMessageMapsSize(t *testing.T, tracker *DoubleSigningTrackerImpl,
	prePrepareCount, prepareCount, commitCount, roundChangeCount int,
	view *ibftProto.View, sender types.Address) {
	t.Helper()

	prePrepareMsgs := tracker.preprepare.getSenderMsgsLocked(view, sender)
	prepareMsgs := tracker.prepare.getSenderMsgsLocked(view, sender)
	commitMsgs := tracker.commit.getSenderMsgsLocked(view, sender)
	roundChangeMsgs := tracker.roundChange.getSenderMsgsLocked(view, sender)

	require.Len(t, prePrepareMsgs, prePrepareCount)
	require.Len(t, prepareMsgs, prepareCount)
	require.Len(t, commitMsgs, commitCount)
	require.Len(t, roundChangeMsgs, roundChangeCount)
}
