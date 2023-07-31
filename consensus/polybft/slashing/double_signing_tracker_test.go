package slashing

import (
	"fmt"
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

	proposalHash := generateRandomProposalHash(t)
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
	tracker.Handle(commitMsg)
	tracker.Handle(roundChangeMsg)

	prepareMsgs = tracker.prepare.getSenderMsgsLocked(view, sender)
	commitMsgs := tracker.commit.getSenderMsgsLocked(view, sender)
	roundChangeMsgs := tracker.roundChange.getSenderMsgsLocked(view, sender)

	assertSenderMessageMapsSize(t, tracker, 0, 1, 1, 1, view, sender)

	require.Equal(t, prepareMsg, prepareMsgs[0])
	require.Equal(t, commitMsg, commitMsgs[0])
	require.Equal(t, roundChangeMsg, roundChangeMsgs[0])
}

func TestDoubleSigningTracker_Handle_MultipleSenders(t *testing.T) {
	t.Parallel()

	const (
		heightsCount = 5
		sendersCount = 4
	)

	tracker := NewDoubleSigningTracker(hclog.NewNullLogger())

	keys := make([]*wallet.Key, sendersCount)
	for i := 0; i < len(keys); i++ {
		acc, err := wallet.GenerateAccount()
		require.NoError(t, err)

		keys[i] = wallet.NewKey(acc)
	}

	expectedPrePrepare := make(map[types.Address][]*ibftProto.Message, sendersCount*heightsCount)
	expectedPrepare := make(map[types.Address][]*ibftProto.Message, sendersCount*heightsCount)
	expectedCommit := make(map[types.Address][]*ibftProto.Message, sendersCount*heightsCount)
	expectedRoundChange := make(map[types.Address][]*ibftProto.Message, sendersCount*heightsCount)

	for _, k := range keys {
		expectedPrePrepare[types.Address(k.Address())] = make([]*ibftProto.Message, 0, heightsCount)
		expectedPrepare[types.Address(k.Address())] = make([]*ibftProto.Message, 0, heightsCount)
		expectedCommit[types.Address(k.Address())] = make([]*ibftProto.Message, 0, heightsCount)
		expectedRoundChange[types.Address(k.Address())] = make([]*ibftProto.Message, 0, heightsCount)

		for i := uint64(1); i <= heightsCount; i++ {
			proposalHash := generateRandomProposalHash(t)
			view := &ibftProto.View{Height: i, Round: 1}

			prePrepare := buildPrePrepareMessage(t, view, k, proposalHash)
			prepare := buildPrepareMessage(t, view, k, proposalHash)
			commit := buildCommitMessage(t, view, k, proposalHash)
			roundChange := buildRoundChangeMessage(t, view, k)

			expectedPrePrepare[types.Address(k.Address())] = append(expectedPrePrepare[types.Address(k.Address())], prePrepare)
			expectedPrepare[types.Address(k.Address())] = append(expectedPrepare[types.Address(k.Address())], prepare)
			expectedCommit[types.Address(k.Address())] = append(expectedCommit[types.Address(k.Address())], commit)
			expectedRoundChange[types.Address(k.Address())] = append(expectedRoundChange[types.Address(k.Address())], roundChange)

			tracker.Handle(prePrepare)
			tracker.Handle(prepare)
			tracker.Handle(commit)
			tracker.Handle(roundChange)
		}
	}

	for _, k := range keys {
		sender := types.Address(k.Address())
		expPrePrepares := expectedPrePrepare[sender]
		expPrepares := expectedPrepare[sender]
		expCommits := expectedCommit[sender]
		expRoundChanges := expectedRoundChange[sender]

		for i := uint64(0); i < heightsCount; i++ {
			view := &ibftProto.View{Height: i + 1, Round: 1}
			actualPrePrepares := tracker.preprepare.getSenderMsgsLocked(view, sender)
			actualPrepares := tracker.prepare.getSenderMsgsLocked(view, sender)
			actualCommits := tracker.commit.getSenderMsgsLocked(view, sender)
			actualRoundChanges := tracker.roundChange.getSenderMsgsLocked(view, sender)

			assertSenderMessageMapsSize(t, tracker, 1, 1, 1, 1, view, sender)
			require.Equal(t, expPrePrepares[i], actualPrePrepares[0])
			require.Equal(t, expPrepares[i], actualPrepares[0])
			require.Equal(t, expCommits[i], actualCommits[0])
			require.Equal(t, expRoundChanges[i], actualRoundChanges[0])
		}
	}
}

func TestDoubleSigningTracker_validateMessage(t *testing.T) {
	t.Parallel()

	acc, err := wallet.GenerateAccount()
	require.NoError(t, err)

	key := wallet.NewKey(acc)

	cases := []struct {
		name        string
		initHandler func(tracker *DoubleSigningTrackerImpl)
		msgBuilder  func() *ibftProto.Message
		errMsg      string
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
			name: "invalid message spammer",
			msgBuilder: func() *ibftProto.Message {
				return buildCommitMessage(t, &ibftProto.View{Height: 1, Round: 4}, key, types.ZeroHash)
			},
			initHandler: func(tracker *DoubleSigningTrackerImpl) {
				view := &ibftProto.View{Height: 1, Round: 4}
				tracker.commit.addMessage(view, types.Address(key.Address()),
					buildCommitMessage(t, view, key, types.ZeroHash))
			},
			errMsg: fmt.Sprintf("sender %s is detected as a spammer", key.Address()),
		},
		{
			name: "valid message",
			msgBuilder: func() *ibftProto.Message {
				proposalHash := generateRandomProposalHash(t)
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
			if c.initHandler != nil {
				c.initHandler(tracker)
			}
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

	proposalHash := generateRandomProposalHash(t)
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

func TestDoubleSigningTracker_GetEvidences(t *testing.T) {
	t.Parallel()

	const (
		prepareMsgsCount = 2
		commitMsgsCount  = 3
	)

	acc, err := wallet.GenerateAccount()
	require.NoError(t, err)

	key := wallet.NewKey(acc)
	view := &ibftProto.View{Height: 6, Round: 2}
	tracker := NewDoubleSigningTracker(hclog.NewNullLogger())

	tracker.Handle(buildPrePrepareMessage(t, view, key, generateRandomProposalHash(t)))

	prepareMessages := make([]*ibftProto.Message, 0, prepareMsgsCount)
	commitMessages := make([]*ibftProto.Message, 0, commitMsgsCount)

	for i := 0; i < prepareMsgsCount; i++ {
		msg := buildPrepareMessage(t, view, key, generateRandomProposalHash(t))
		prepareMessages = append(prepareMessages, msg)
		tracker.Handle(msg)
	}

	for i := 0; i < commitMsgsCount; i++ {
		msg := buildCommitMessage(t, view, key, generateRandomProposalHash(t))
		commitMessages = append(commitMessages, msg)
		tracker.Handle(msg)
	}

	evidences := tracker.GetEvidences(view.Height)
	require.Len(t, evidences, 2)

	prepareEvidence := evidences[0]
	require.Len(t, prepareEvidence.messages, len(prepareMessages))

	commitEvidence := evidences[1]
	require.Len(t, commitEvidence.messages, len(commitMessages))

	for i, msg := range prepareMessages {
		t.Log("Index", i)
		require.Equal(t, msg, prepareEvidence.messages[i])
	}

	for i, msg := range commitMessages {
		t.Log("Index", i)
		require.Equal(t, msg, commitEvidence.messages[i])
	}
}
