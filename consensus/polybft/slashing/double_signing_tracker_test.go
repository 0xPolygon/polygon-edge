package slashing

import (
	"testing"

	ibftProto "github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
)

func TestDoubleSigningTracker_Handle_SingleSender(t *testing.T) {
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

	sender := types.Address(key.Address())

	prePrepareMsgs := tracker.preprepare.getSenderMsgsLocked(prePrepareView, sender)
	assertSenderMessageMapsSize(t, tracker, 1, 0, 0, 0, prePrepareView, sender)
	require.Equal(t, prePrepareMsg, prePrepareMsgs[0])

	prepareMsgs := tracker.prepare.getSenderMsgsLocked(view, sender)
	assertSenderMessageMapsSize(t, tracker, 0, 1, 0, 0, view, sender)
	require.Equal(t, prepareMsg, prepareMsgs[0])

	view.Round++
	prepareMsg = buildPrepareMessage(t, view, key, proposalHash)
	commitMsg := buildCommitMessage(t, view, key, proposalHash)
	roundChangeMsg := buildRoundChangeMessage(t, view, key)

	tracker.Handle(prepareMsg)
	tracker.Handle(prepareMsg)
	tracker.Handle(commitMsg)
	tracker.Handle(roundChangeMsg)

	prepareMsgs = tracker.prepare.getSenderMsgsLocked(view, sender)
	commitMsgs := tracker.commit.getSenderMsgsLocked(view, sender)
	roundChangeMsgs := tracker.roundChange.getSenderMsgsLocked(view, sender)

	assertSenderMessageMapsSize(t, tracker, 0, 2, 1, 1, view, sender)

	require.Equal(t, prepareMsg, prepareMsgs[0])
	require.Equal(t, prepareMsg, prepareMsgs[1])
	require.Equal(t, commitMsg, commitMsgs[0])
	require.Equal(t, roundChangeMsg, roundChangeMsgs[0])
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

func TestDoubleSigningTracker_PruneMsgsUntil(t *testing.T) {
	t.Parallel()

	acc, err := wallet.GenerateAccount()
	require.NoError(t, err)

	key := wallet.NewKey(acc)

	proposalHash := types.StringToHash("dummy proposal")
	view := &ibftProto.View{Height: 1, Round: 1}

	tracker := NewDoubleSigningTracker(hclog.NewNullLogger())
	tracker.Handle(buildPrePrepareMessage(t, view, key, proposalHash))
	tracker.Handle(buildPrepareMessage(t, view, key, proposalHash))
	tracker.Handle(buildCommitMessage(t, view, key, proposalHash))

	view.Height = 3
	tracker.Handle(buildPrePrepareMessage(t, view, key, proposalHash))
	tracker.Handle(buildPrepareMessage(t, view, key, proposalHash))
	tracker.Handle(buildCommitMessage(t, view, key, proposalHash))

	view = &ibftProto.View{Height: 5, Round: 1}
	tracker.Handle(buildPrePrepareMessage(t, view, key, proposalHash))
	tracker.Handle(buildPrepareMessage(t, view, key, proposalHash))
	tracker.Handle(buildCommitMessage(t, view, key, proposalHash))

	view.Round = 4
	tracker.Handle(buildCommitMessage(t, view, key, proposalHash))

	tracker.PruneMsgsUntil(5)

	sender := types.Address(key.Address())

	for height := uint64(0); height < view.Height; height++ {
		for round := uint64(0); round < view.Round; round++ {
			currentView := &ibftProto.View{Height: height, Round: round}
			assertSenderMessageMapsSize(t, tracker, 0, 0, 0, 0, currentView, sender)
		}
	}
}
