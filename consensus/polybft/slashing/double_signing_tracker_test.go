package slashing

import (
	"testing"

	ibftProto "github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
)

func TestDoubleSigningTracker_Handle(t *testing.T) {
	t.Parallel()

	acc, err := wallet.GenerateAccount()
	require.NoError(t, err)

	key := wallet.NewKey(acc)

	proposalHash := types.StringToHash("Hello World.")
	prePrepareView := &ibftProto.View{Height: 6, Round: 1}
	view := &ibftProto.View{Height: 8, Round: 1}

	prePrepareMsg := buildPrePrepareMessage(t, prePrepareView, key, proposalHash)
	prepareMsg := buildPrepareMessage(t, view, key, proposalHash)

	tracker := NewDoubleSigningTracker(hclog.NewNullLogger())
	tracker.Handle(prePrepareMsg)
	tracker.Handle(prepareMsg)

	prePrepareMsgs := tracker.preprepare.getSenderMsgsLocked(prePrepareView, types.Address(key.Address()))
	require.Len(t, prePrepareMsgs, 1)
	require.Equal(t, prePrepareMsg, prePrepareMsgs[0])
	require.Empty(t, tracker.prepare.getSenderMsgsLocked(prePrepareView, types.Address(key.Address())))
	require.Empty(t, tracker.commit.getSenderMsgsLocked(prePrepareView, types.Address(key.Address())))
	require.Empty(t, tracker.roundChange.getSenderMsgsLocked(prePrepareView, types.Address(key.Address())))

	prepareMsgs := tracker.prepare.getSenderMsgsLocked(view, types.Address(key.Address()))
	require.Len(t, prepareMsgs, 1)
	require.Equal(t, prepareMsg, prepareMsgs[0])
	require.Empty(t, tracker.preprepare.getSenderMsgsLocked(view, types.Address(key.Address())))
	require.Empty(t, tracker.commit.getSenderMsgsLocked(view, types.Address(key.Address())))
	require.Empty(t, tracker.roundChange.getSenderMsgsLocked(view, types.Address(key.Address())))

	view.Round++
	prepareMsg = buildPrepareMessage(t, view, key, proposalHash)
	commitMsg := buildCommitMessage(t, view, key, proposalHash)
	roundChangeMsg := buildRoundChangeMessage(t, view, key)

	tracker.Handle(prepareMsg)
	tracker.Handle(prepareMsg)
	tracker.Handle(commitMsg)
	tracker.Handle(roundChangeMsg)

	prepareMsgs = tracker.prepare.getSenderMsgsLocked(view, types.Address(key.Address()))
	commitMsgs := tracker.commit.getSenderMsgsLocked(view, types.Address(key.Address()))
	roundChangeMsgs := tracker.roundChange.getSenderMsgsLocked(view, types.Address(key.Address()))

	require.Len(t, prepareMsgs, 2)
	require.Equal(t, prepareMsg, prepareMsgs[0])
	require.Equal(t, prepareMsg, prepareMsgs[1])
	require.Len(t, commitMsgs, 1)
	require.Equal(t, commitMsg, commitMsgs[0])
	require.Len(t, roundChangeMsgs, 1)
	require.Equal(t, roundChangeMsg, roundChangeMsgs[0])
	require.Empty(t, tracker.preprepare.getSenderMsgsLocked(view, types.Address(key.Address())))
}

func TestDoubleSigningTracker_validateMessage(t *testing.T) {
	t.Parallel()

	acc, err := wallet.GenerateAccount()
	require.NoError(t, err)

	key := wallet.NewKey(acc)

	cases := []struct {
		name       string
		msgBuilder func() *ibftProto.Message
		errMsg     string
	}{
		{
			name: "invalid message view undefined",
			msgBuilder: func() *ibftProto.Message {
				return &ibftProto.Message{}
			},
			errMsg: errViewUndefined.Error(),
		},
		{
			name: "invalid message invalid message type",
			msgBuilder: func() *ibftProto.Message {
				return &ibftProto.Message{View: &ibftProto.View{Height: 1, Round: 4}, Type: 6}
			},
			errMsg: errInvalidMsgType.Error(),
		},
		{
			name: "invalid message invalid signature",
			msgBuilder: func() *ibftProto.Message {
				return &ibftProto.Message{
					View: &ibftProto.View{Height: 1, Round: 4},
					Type: ibftProto.MessageType_COMMIT,
				}
			},
			errMsg: "failed to recover address from signature",
		},
		{
			name: "invalid message invalid signature",
			msgBuilder: func() *ibftProto.Message {
				msg := &ibftProto.Message{
					View: &ibftProto.View{Height: 1, Round: 4},
					Type: ibftProto.MessageType_COMMIT,
					From: types.ZeroAddress.Bytes(),
				}

				msg, err := key.SignIBFTMessage(msg)
				require.NoError(t, err)

				return msg
			},
			errMsg: errSignerAndSenderMismatch.Error(),
		},
		{
			name: "valid message",
			msgBuilder: func() *ibftProto.Message {
				proposalHash := types.StringToHash("Lorem Ipsum")
				committedSeal, err := key.Sign(proposalHash.Bytes())
				require.NoError(t, err)

				msg := &ibftProto.Message{
					View: &ibftProto.View{Height: 1, Round: 4},
					Type: ibftProto.MessageType_COMMIT,
					From: key.Address().Bytes(),
					Payload: &ibftProto.Message_CommitData{
						CommitData: &ibftProto.CommitMessage{
							ProposalHash:  proposalHash.Bytes(),
							CommittedSeal: committedSeal,
						},
					},
				}

				msg, err = key.SignIBFTMessage(msg)
				require.NoError(t, err)

				return msg
			},
			errMsg: "",
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			tracker := NewDoubleSigningTracker(hclog.NewNullLogger())
			msg := c.msgBuilder()

			err := tracker.validateMsg(msg)
			if c.errMsg != "" {
				require.ErrorContains(t, err, c.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

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
