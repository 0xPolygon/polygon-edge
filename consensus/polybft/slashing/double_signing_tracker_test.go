package slashing

import (
	"bytes"
	"fmt"
	"sort"
	"sync"
	"testing"

	ibftProto "github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"pgregory.net/rapid"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/common"
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

	tracker, err := NewDoubleSigningTracker(hclog.NewNullLogger(),
		&dummyValidatorsProvider{accounts: []*wallet.Account{acc}})
	require.NoError(t, err)

	tracker.Handle(prePrepareMsg)
	tracker.Handle(prepareMsg)

	sender := types.Address(key.Address())

	prePrepareMsgs := tracker.preprepare.getSenderMsgs(prePrepareView, sender)
	assertSenderMessageMapsSize(t, tracker, 1, 0, 0, 0, prePrepareView, sender)
	require.Equal(t, prePrepareMsg, prePrepareMsgs[0])

	prepareMsgs := tracker.prepare.getSenderMsgs(view, sender)
	assertSenderMessageMapsSize(t, tracker, 0, 1, 0, 0, view, sender)
	require.Equal(t, prepareMsg, prepareMsgs[0])

	view.Round++
	prepareMsg = buildPrepareMessage(t, view, key, proposalHash)
	commitMsg := buildCommitMessage(t, view, key, proposalHash)
	roundChangeMsg := buildRoundChangeMessage(t, view, key)

	tracker.Handle(prepareMsg)
	tracker.Handle(commitMsg)
	tracker.Handle(roundChangeMsg)

	prepareMsgs = tracker.prepare.getSenderMsgs(view, sender)
	commitMsgs := tracker.commit.getSenderMsgs(view, sender)
	roundChangeMsgs := tracker.roundChange.getSenderMsgs(view, sender)

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

	keys := make([]*wallet.Key, sendersCount)
	accounts := make([]*wallet.Account, sendersCount)

	for i := 0; i < len(keys); i++ {
		acc, err := wallet.GenerateAccount()
		require.NoError(t, err)

		keys[i] = wallet.NewKey(acc)
		accounts[i] = acc
	}

	tracker, err := NewDoubleSigningTracker(hclog.NewNullLogger(), &dummyValidatorsProvider{accounts: accounts})
	require.NoError(t, err)

	expectedPrePrepare := make(map[types.Address][]*ibftProto.Message, sendersCount*heightsCount)
	expectedPrepare := make(map[types.Address][]*ibftProto.Message, sendersCount*heightsCount)
	expectedCommit := make(map[types.Address][]*ibftProto.Message, sendersCount*heightsCount)
	expectedRoundChange := make(map[types.Address][]*ibftProto.Message, sendersCount*heightsCount)

	msgCh := make(chan *ibftProto.Message, 4)

	var wg sync.WaitGroup

	go func() {
		wg.Add(1)
		defer wg.Done()

		for msg := range msgCh {
			tracker.Handle(msg)
		}
	}()

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

			msgCh <- prePrepare
			msgCh <- prepare
			msgCh <- commit
			msgCh <- roundChange
		}
	}

	close(msgCh)
	wg.Wait()

	for _, k := range keys {
		sender := types.Address(k.Address())
		expPrePrepares := expectedPrePrepare[sender]
		expPrepares := expectedPrepare[sender]
		expCommits := expectedCommit[sender]
		expRoundChanges := expectedRoundChange[sender]

		for i := uint64(0); i < heightsCount; i++ {
			view := &ibftProto.View{Height: i + 1, Round: 1}
			actualPrePrepares := tracker.preprepare.getSenderMsgs(view, sender)
			actualPrepares := tracker.prepare.getSenderMsgs(view, sender)
			actualCommits := tracker.commit.getSenderMsgs(view, sender)
			actualRoundChanges := tracker.roundChange.getSenderMsgs(view, sender)

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

	const keysCount = 2

	allAccounts := make([]*wallet.Account, keysCount)
	allKeys := make([]*wallet.Key, keysCount)

	for i := 0; i < keysCount; i++ {
		acc, err := wallet.GenerateAccount()
		require.NoError(t, err)

		allAccounts[i] = acc
		key := wallet.NewKey(acc)
		allKeys[i] = key
	}

	cases := []struct {
		name        string
		initHandler func(tracker *DoubleSigningTrackerImpl)
		msgBuilder  func() *ibftProto.Message
		accounts    []*wallet.Account
		errMsg      string
	}{
		{
			name: "invalid message (view undefined)",
			msgBuilder: func() *ibftProto.Message {
				return &ibftProto.Message{}
			},
			errMsg: errViewUndefined.Error(),
		},
		{
			name: "invalid message (invalid message type)",
			msgBuilder: func() *ibftProto.Message {
				return &ibftProto.Message{View: &ibftProto.View{Height: 1, Round: 4}, Type: 6}
			},
			errMsg: errInvalidMsgType.Error(),
		},
		{
			name: "invalid message (invalid signature)",
			msgBuilder: func() *ibftProto.Message {
				return &ibftProto.Message{
					View: &ibftProto.View{Height: 1, Round: 4},
					Type: ibftProto.MessageType_COMMIT,
				}
			},
			errMsg: "failed to recover address from signature",
		},
		{
			name: "invalid message (signer and sender mismatch)",
			msgBuilder: func() *ibftProto.Message {
				msg := &ibftProto.Message{
					View: &ibftProto.View{Height: 1, Round: 4},
					Type: ibftProto.MessageType_COMMIT,
					From: allKeys[1].Address().Bytes(),
				}

				msg, err := allKeys[0].SignIBFTMessage(msg)
				require.NoError(t, err)

				return msg
			},
			errMsg: errSignerAndSenderMismatch.Error(),
		},
		{
			name: "invalid message (spammer detected)",
			msgBuilder: func() *ibftProto.Message {
				return buildCommitMessage(t, &ibftProto.View{Height: 1, Round: 4}, allKeys[0], types.ZeroHash)
			},
			initHandler: func(tracker *DoubleSigningTrackerImpl) {
				view := &ibftProto.View{Height: 1, Round: 4}
				tracker.commit.addMessage(view, types.Address(allKeys[0].Address()),
					buildCommitMessage(t, view, allKeys[0], types.ZeroHash))
			},
			errMsg: fmt.Sprintf("sender %s is detected as a spammer", allKeys[0].Address()),
		},
		{
			name: "invalid message (unknown sender)",
			msgBuilder: func() *ibftProto.Message {
				return buildCommitMessage(t, &ibftProto.View{Height: 1, Round: 4}, allKeys[0], types.ZeroHash)
			},
			accounts: allAccounts[1:],
			errMsg:   errUnknownSender.Error(),
		},
		{
			name: "valid message",
			msgBuilder: func() *ibftProto.Message {
				proposalHash := generateRandomProposalHash(t)
				committedSeal, err := allKeys[0].Sign(proposalHash.Bytes())
				require.NoError(t, err)

				msg := &ibftProto.Message{
					View: &ibftProto.View{Height: 1, Round: 4},
					Type: ibftProto.MessageType_COMMIT,
					From: allKeys[0].Address().Bytes(),
					Payload: &ibftProto.Message_CommitData{
						CommitData: &ibftProto.CommitMessage{
							ProposalHash:  proposalHash.Bytes(),
							CommittedSeal: committedSeal,
						},
					},
				}

				msg, err = allKeys[0].SignIBFTMessage(msg)
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

			accounts := allAccounts
			if c.accounts != nil {
				accounts = c.accounts
			}

			tracker, err := NewDoubleSigningTracker(hclog.NewNullLogger(), &dummyValidatorsProvider{accounts: accounts})
			require.NoError(t, err)

			if c.initHandler != nil {
				c.initHandler(tracker)
			}
			msg := c.msgBuilder()

			err = tracker.validateMsg(msg)
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

	const (
		heightsNum = 4
		roundsNum  = 2
	)

	acc, err := wallet.GenerateAccount()
	require.NoError(t, err)

	key := wallet.NewKey(acc)

	proposalHash := generateRandomProposalHash(t)

	tracker, err := NewDoubleSigningTracker(hclog.NewNullLogger(),
		&dummyValidatorsProvider{accounts: []*wallet.Account{acc}})
	require.NoError(t, err)

	views := make([]*ibftProto.View, 0, heightsNum*roundsNum)

	for height := uint64(0); height < heightsNum; height++ {
		for round := uint64(0); round < roundsNum; round++ {
			view := &ibftProto.View{Height: height + 1, Round: round + 1}
			views = append(views, view)
			tracker.Handle(buildPrePrepareMessage(t, view, key, proposalHash))
			tracker.Handle(buildPrepareMessage(t, view, key, proposalHash))
			tracker.Handle(buildCommitMessage(t, view, key, proposalHash))
			tracker.Handle(buildRoundChangeMessage(t, view, key))
		}
	}

	tracker.PruneMsgsUntil(heightsNum)

	sender := types.Address(key.Address())

	for _, view := range views {
		if view.Height < heightsNum {
			assertSenderMessageMapsSize(t, tracker, 0, 0, 0, 0, view, sender)
		} else {
			assertSenderMessageMapsSize(t, tracker, 1, 1, 1, 1, view, sender)
		}
	}
}

func TestDoubleSigningTracker_GetDoubleSigners_SingleDoubleSigner(t *testing.T) {
	t.Parallel()

	const (
		prepareMsgsCount = 5
		commitMsgsCount  = 3
		sendersCount     = 3
		height           = 4
	)

	keys := make([]*wallet.Key, 0, sendersCount)
	rounds := []uint64{1, 3, 2}
	validatorsProvider := &dummyValidatorsProvider{accounts: make([]*wallet.Account, 0, sendersCount)}

	for i := 0; i < sendersCount; i++ {
		acc, err := wallet.GenerateAccount()
		require.NoError(t, err)

		validatorsProvider.accounts = append(validatorsProvider.accounts, acc)
		keys = append(keys, wallet.NewKey(acc))
	}

	doubleSignerAddr := types.Address(keys[0].Address())

	tracker, err := NewDoubleSigningTracker(hclog.NewNullLogger(), validatorsProvider)
	require.NoError(t, err)

	for _, r := range rounds {
		for _, key := range keys {
			view := &ibftProto.View{Height: height, Round: r}

			if key.Address() == ethgo.Address(doubleSignerAddr) {
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
			} else {
				tracker.Handle(buildPrePrepareMessage(t, view, key, generateRandomProposalHash(t)))
				tracker.Handle(buildPrepareMessage(t, view, key, generateRandomProposalHash(t)))
				tracker.Handle(buildCommitMessage(t, view, key, generateRandomProposalHash(t)))
			}
		}
	}

	doubleSigners := tracker.GetDoubleSigners(height)
	require.Len(t, doubleSigners, 1)
	require.Equal(t, doubleSignerAddr, doubleSigners[0])
}

func TestDoubleSigningTracker_GetDoubleSigners_Property(t *testing.T) {
	t.Parallel()

	const (
		maxRound      = 100
		maxHeight     = 1000
		maxMsgs       = 4
		accountsCount = 5
	)

	rapid.Check(t, func(rapidT *rapid.T) {
		doubleSignersCount := rapid.IntRange(0, accountsCount).Draw(rapidT, "double signers count")
		provider := &dummyValidatorsProvider{accounts: make([]*wallet.Account, accountsCount)}

		doubleSignerAddrs := make([]types.Address, 0, doubleSignersCount)
		keys := make([]*wallet.Key, accountsCount)

		for i := 0; i < accountsCount; i++ {
			acc, err := wallet.GenerateAccount()
			require.NoError(t, err)

			keys[i] = wallet.NewKey(acc)
			provider.accounts[i] = acc

			if len(doubleSignerAddrs) < doubleSignersCount {
				doubleSignerAddrs = append(doubleSignerAddrs, types.Address(keys[i].Address()))
			}
		}

		sort.Slice(doubleSignerAddrs, func(i, j int) bool {
			return bytes.Compare(doubleSignerAddrs[i].Bytes(), doubleSignerAddrs[j].Bytes()) < 0
		})

		tracker, err := NewDoubleSigningTracker(hclog.NewNullLogger(), provider)
		require.NoError(t, err)

		heightsNum := rapid.IntRange(1, 5).Draw(rapidT, "number of heights")
		roundsNum := rapid.IntRange(1, 4).Draw(rapidT, "number of rounds")

		heights := make([]uint64, heightsNum)
		expectedDoubleSigning := make(map[uint64]bool, heightsNum)
		existingHeights := make(map[uint64]struct{}, heightsNum)

		for i := 0; i < heightsNum; i++ {
			height := enforceUniqueRandomNumber(rapidT, existingHeights, 1, maxHeight, fmt.Sprintf("generate height#%d", i+1))
			existingHeights[height] = struct{}{}
			shouldDoubleSign := rapid.Bool().Draw(rapidT, "double signing flag")
			expectedDoubleSigning[height] = shouldDoubleSign
			existingRounds := make(map[uint64]struct{}, roundsNum)
			proposalHash := generateRandomProposalHash(t)

			for j := 0; j < roundsNum; j++ {
				round := enforceUniqueRandomNumber(rapidT, existingRounds, 0, maxRound, fmt.Sprintf("round#%d", j+1))
				existingRounds[round] = struct{}{}
				heights[i] = height

				view := &ibftProto.View{Height: height, Round: round}

				if shouldDoubleSign {
					messagesCount := rapid.IntRange(2, maxMsgs).Draw(rapidT, "messages count")

					for i, key := range keys {
						if i < len(doubleSignerAddrs) {
							for msgsCount := 0; msgsCount < messagesCount; msgsCount++ {
								tracker.Handle(buildPrepareMessage(t, view, key, generateRandomProposalHash(t)))
							}
						} else {
							tracker.Handle(buildPrepareMessage(t, view, key, proposalHash))
						}
					}
				} else {
					for _, key := range keys {
						tracker.Handle(buildPrepareMessage(t, view, key, proposalHash))
					}
				}
			}
		}

		for _, height := range heights {
			doubleSigners := tracker.GetDoubleSigners(height)
			if expectedDoubleSigning[height] {
				if len(doubleSigners) != len(doubleSignerAddrs) {
					t.Log("failed at height", height)
					t.Log(tracker.prepare.String())
				}

				require.Len(t, doubleSigners, len(doubleSignerAddrs))
				require.Equal(t, doubleSigners, DoubleSigners(doubleSignerAddrs))
			} else {
				if len(doubleSigners) > 0 {
					t.Log("failed at height", height)
					t.Log(tracker.prepare.String())
				}

				require.Empty(t, doubleSigners)
			}
		}
	})
}

func TestDoubleSigningTracker_PostBlock(t *testing.T) {
	t.Parallel()

	const validatorsCount = 4

	validatorAccs := make([]*wallet.Account, validatorsCount)

	for i := 0; i < validatorsCount; i++ {
		acc, err := wallet.GenerateAccount()
		require.NoError(t, err)

		validatorAccs[i] = acc
	}

	provider := &dummyValidatorsProvider{accounts: validatorAccs}
	tracker, err := NewDoubleSigningTracker(hclog.NewNullLogger(), provider)
	require.NoError(t, err)

	require.NoError(t, tracker.PostBlock(
		&common.PostBlockRequest{
			FullBlock: &types.FullBlock{
				Block: &types.Block{
					Header: &types.Header{Number: 1},
				},
			},
		}))

	validators, err := provider.GetAllValidators()
	require.NoError(t, err)

	require.Equal(t, tracker.validators, validators)
}
