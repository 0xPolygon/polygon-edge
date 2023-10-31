package polybft

import (
	"math/big"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/testutil"
	"google.golang.org/protobuf/proto"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/merkle-tree"
	"github.com/0xPolygon/polygon-edge/types"
)

func newTestStateSyncManager(t *testing.T, key *validator.TestValidator, runtime Runtime) *stateSyncManager {
	t.Helper()

	tmpDir, err := os.MkdirTemp("/tmp", "test-data-dir-state-sync")
	require.NoError(t, err)

	state := newTestState(t)
	require.NoError(t, state.EpochStore.insertEpoch(0, nil))

	topic := &mockTopic{}

	s := newStateSyncManager(hclog.NewNullLogger(), state,
		&stateSyncConfig{
			stateSenderAddr:   types.Address{},
			jsonrpcAddr:       "",
			dataDir:           tmpDir,
			topic:             topic,
			key:               key.Key(),
			maxCommitmentSize: maxCommitmentSize,
		}, runtime)

	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	return s
}

func TestStateSyncManager_PostEpoch_BuildCommitment(t *testing.T) {
	t.Parallel()

	vals := validator.NewTestValidators(t, 5)

	t.Run("When node is validator", func(t *testing.T) {
		t.Parallel()

		s := newTestStateSyncManager(t, vals.GetValidator("0"), &mockRuntime{isActiveValidator: true})

		// there are no state syncs
		require.NoError(t, s.buildCommitment(nil))
		require.Nil(t, s.pendingCommitments)

		stateSyncs10 := generateStateSyncEvents(t, 10, 0)

		// add 5 state syncs starting in index 0, it will generate one smaller commitment
		for i := 0; i < 5; i++ {
			require.NoError(t, s.state.StateSyncStore.insertStateSyncEvent(stateSyncs10[i]))
		}

		require.NoError(t, s.buildCommitment(nil))
		require.Len(t, s.pendingCommitments, 1)
		require.Equal(t, uint64(0), s.pendingCommitments[0].StartID.Uint64())
		require.Equal(t, uint64(4), s.pendingCommitments[0].EndID.Uint64())
		require.Equal(t, uint64(0), s.pendingCommitments[0].Epoch)

		// add the next 5 state syncs, at that point, so that it generates a larger commitment
		for i := 5; i < 10; i++ {
			require.NoError(t, s.state.StateSyncStore.insertStateSyncEvent(stateSyncs10[i]))
		}

		require.NoError(t, s.buildCommitment(nil))
		require.Len(t, s.pendingCommitments, 2)
		require.Equal(t, uint64(0), s.pendingCommitments[1].StartID.Uint64())
		require.Equal(t, uint64(9), s.pendingCommitments[1].EndID.Uint64())
		require.Equal(t, uint64(0), s.pendingCommitments[1].Epoch)

		// the message was sent
		require.NotNil(t, s.config.topic.(*mockTopic).consume()) //nolint
	})

	t.Run("When node is not validator", func(t *testing.T) {
		t.Parallel()

		s := newTestStateSyncManager(t, vals.GetValidator("0"), &mockRuntime{isActiveValidator: false})

		stateSyncs10 := generateStateSyncEvents(t, 10, 0)

		// add 5 state syncs starting in index 0, they will be saved to db
		for i := 0; i < 5; i++ {
			require.NoError(t, s.state.StateSyncStore.insertStateSyncEvent(stateSyncs10[i]))
		}

		// I am not a validator so no commitments should be built
		require.NoError(t, s.buildCommitment(nil))
		require.Len(t, s.pendingCommitments, 0)
	})
}

func TestStateSyncManager_MessagePool(t *testing.T) {
	t.Parallel()

	vals := validator.NewTestValidators(t, 5)

	t.Run("Old epoch", func(t *testing.T) {
		t.Parallel()

		s := newTestStateSyncManager(t, vals.GetValidator("0"), &mockRuntime{isActiveValidator: true})

		s.epoch = 1
		msg := &TransportMessage{
			EpochNumber: 0,
		}

		err := s.saveVote(msg)
		require.NoError(t, err)
	})

	t.Run("Sender is not a validator", func(t *testing.T) {
		t.Parallel()

		s := newTestStateSyncManager(t, vals.GetValidator("0"), &mockRuntime{isActiveValidator: true})
		s.validatorSet = vals.ToValidatorSet()

		badVal := validator.NewTestValidator(t, "a", 0)
		msg, err := newMockMsg().sign(badVal, signer.DomainStateReceiver)
		require.NoError(t, err)

		require.Error(t, s.saveVote(msg))
	})

	t.Run("Invalid epoch", func(t *testing.T) {
		t.Parallel()

		s := newTestStateSyncManager(t, vals.GetValidator("0"), &mockRuntime{isActiveValidator: true})
		s.validatorSet = vals.ToValidatorSet()

		val := newMockMsg()
		msg, err := val.sign(vals.GetValidator("0"), signer.DomainStateReceiver)
		require.NoError(t, err)

		msg.EpochNumber = 1

		require.NoError(t, s.saveVote(msg))

		// no votes for the current epoch
		votes, err := s.state.StateSyncStore.getMessageVotes(0, msg.Hash)
		require.NoError(t, err)
		require.Len(t, votes, 0)

		// returns an error for the invalid epoch
		_, err = s.state.StateSyncStore.getMessageVotes(1, msg.Hash)
		require.Error(t, err)
	})

	t.Run("Sender and signature mismatch", func(t *testing.T) {
		t.Parallel()

		s := newTestStateSyncManager(t, vals.GetValidator("0"), &mockRuntime{isActiveValidator: true})
		s.validatorSet = vals.ToValidatorSet()

		// validator signs the msg in behalf of another validator
		val := newMockMsg()
		msg, err := val.sign(vals.GetValidator("0"), signer.DomainStateReceiver)
		require.NoError(t, err)

		msg.From = vals.GetValidator("1").Address().String()
		require.Error(t, s.saveVote(msg))

		// non validator signs the msg in behalf of a validator
		badVal := validator.NewTestValidator(t, "a", 0)
		msg, err = newMockMsg().sign(badVal, signer.DomainStateReceiver)
		require.NoError(t, err)

		msg.From = vals.GetValidator("1").Address().String()
		require.Error(t, s.saveVote(msg))
	})

	t.Run("Sender votes", func(t *testing.T) {
		t.Parallel()

		s := newTestStateSyncManager(t, vals.GetValidator("0"), &mockRuntime{isActiveValidator: true})
		s.validatorSet = vals.ToValidatorSet()

		msg := newMockMsg()
		val1signed, err := msg.sign(vals.GetValidator("1"), signer.DomainStateReceiver)
		require.NoError(t, err)

		val2signed, err := msg.sign(vals.GetValidator("2"), signer.DomainStateReceiver)
		require.NoError(t, err)

		// vote with validator 1
		require.NoError(t, s.saveVote(val1signed))

		votes, err := s.state.StateSyncStore.getMessageVotes(0, msg.hash)
		require.NoError(t, err)
		require.Len(t, votes, 1)

		// vote with validator 1 again (the votes do not increase)
		require.NoError(t, s.saveVote(val1signed))
		votes, _ = s.state.StateSyncStore.getMessageVotes(0, msg.hash)
		require.Len(t, votes, 1)

		// vote with validator 2
		require.NoError(t, s.saveVote(val2signed))
		votes, _ = s.state.StateSyncStore.getMessageVotes(0, msg.hash)
		require.Len(t, votes, 2)
	})
}

func TestStateSyncManager_BuildCommitment(t *testing.T) {
	vals := validator.NewTestValidators(t, 5)

	s := newTestStateSyncManager(t, vals.GetValidator("0"), &mockRuntime{isActiveValidator: true})
	s.validatorSet = vals.ToValidatorSet()

	// commitment is empty
	commitment, err := s.Commitment(1)
	require.NoError(t, err)
	require.Nil(t, commitment)

	tree, err := merkle.NewMerkleTree([][]byte{{0x1}})
	require.NoError(t, err)

	s.pendingCommitments = []*PendingCommitment{
		{
			MerkleTree: tree,
			StateSyncCommitment: &contractsapi.StateSyncCommitment{
				Root:    tree.Hash(),
				StartID: big.NewInt(0),
				EndID:   big.NewInt(1),
			},
		},
	}

	hash, err := s.pendingCommitments[0].Hash()
	require.NoError(t, err)

	msg := newMockMsg().WithHash(hash.Bytes())

	// validators 0 and 1 vote for the proposal, there is not enough
	// voting power for the proposal
	signedMsg1, err := msg.sign(vals.GetValidator("0"), signer.DomainStateReceiver)
	require.NoError(t, err)

	signedMsg2, err := msg.sign(vals.GetValidator("1"), signer.DomainStateReceiver)
	require.NoError(t, err)

	require.NoError(t, s.saveVote(signedMsg1))
	require.NoError(t, s.saveVote(signedMsg2))

	commitment, err = s.Commitment(1)
	require.NoError(t, err) // there is no error if quorum is not met, since its a valid case
	require.Nil(t, commitment)

	// validator 2 and 3 vote for the proposal, there is enough voting power now

	signedMsg1, err = msg.sign(vals.GetValidator("2"), signer.DomainStateReceiver)
	require.NoError(t, err)

	signedMsg2, err = msg.sign(vals.GetValidator("3"), signer.DomainStateReceiver)
	require.NoError(t, err)

	require.NoError(t, s.saveVote(signedMsg1))
	require.NoError(t, s.saveVote(signedMsg2))

	commitment, err = s.Commitment(1)
	require.NoError(t, err)
	require.NotNil(t, commitment)
}

func TestStateSyncerManager_BuildProofs(t *testing.T) {
	vals := validator.NewTestValidators(t, 5)

	s := newTestStateSyncManager(t, vals.GetValidator("0"), &mockRuntime{isActiveValidator: true})

	for _, evnt := range generateStateSyncEvents(t, 20, 0) {
		require.NoError(t, s.state.StateSyncStore.insertStateSyncEvent(evnt))
	}

	require.NoError(t, s.buildCommitment(nil))
	require.Len(t, s.pendingCommitments, 1)

	mockMsg := &CommitmentMessageSigned{
		Message: &contractsapi.StateSyncCommitment{
			StartID: s.pendingCommitments[0].StartID,
			EndID:   s.pendingCommitments[0].EndID,
		},
	}

	txData, err := mockMsg.EncodeAbi()
	require.NoError(t, err)

	tx := createStateTransactionWithData(1, types.Address{}, txData)

	req := &PostBlockRequest{
		FullBlock: &types.FullBlock{
			Block: &types.Block{
				Transactions: []*types.Transaction{tx},
			},
		},
	}

	require.NoError(t, s.PostBlock(req))
	require.Equal(t, mockMsg.Message.EndID.Uint64()+1, s.nextCommittedIndex)

	for i := uint64(0); i < 10; i++ {
		proof, err := s.state.StateSyncStore.getStateSyncProof(i)
		require.NoError(t, err)
		require.NotNil(t, proof)
	}
}

func TestStateSyncerManager_RemoveProcessedEventsAndProofs(t *testing.T) {
	const stateSyncEventsCount = 5

	vals := validator.NewTestValidators(t, 5)

	s := newTestStateSyncManager(t, vals.GetValidator("0"), &mockRuntime{isActiveValidator: true})
	stateSyncEvents := generateStateSyncEvents(t, stateSyncEventsCount, 0)

	for _, event := range stateSyncEvents {
		require.NoError(t, s.state.StateSyncStore.insertStateSyncEvent(event))
	}

	require.NoError(t, s.buildProofs(&contractsapi.StateSyncCommitment{
		StartID: stateSyncEvents[0].ID,
		EndID:   stateSyncEvents[len(stateSyncEvents)-1].ID,
	}, nil))

	stateSyncEventsBefore, err := s.state.StateSyncStore.list()
	require.NoError(t, err)
	require.Equal(t, stateSyncEventsCount, len(stateSyncEventsBefore))

	for _, event := range stateSyncEvents {
		proof, err := s.state.StateSyncStore.getStateSyncProof(event.ID.Uint64())
		require.NoError(t, err)
		require.NotNil(t, proof)
	}

	for _, event := range stateSyncEvents {
		eventLog := createTestLogForStateSyncResultEvent(t, event.ID.Uint64())
		require.NoError(t, s.ProcessLog(&types.Header{Number: 10}, convertLog(eventLog), nil))
	}

	// all state sync events and their proofs should be removed from the store
	stateSyncEventsAfter, err := s.state.StateSyncStore.list()
	require.NoError(t, err)
	require.Equal(t, 0, len(stateSyncEventsAfter))

	for _, event := range stateSyncEvents {
		proof, err := s.state.StateSyncStore.getStateSyncProof(event.ID.Uint64())
		require.NoError(t, err)
		require.Nil(t, proof)
	}
}

func TestStateSyncerManager_AddLog_BuildCommitments(t *testing.T) {
	t.Parallel()

	vals := validator.NewTestValidators(t, 5)

	t.Run("Node is a validator", func(t *testing.T) {
		t.Parallel()

		s := newTestStateSyncManager(t, vals.GetValidator("0"), &mockRuntime{isActiveValidator: true})

		// empty log which is not an state sync
		require.NoError(t, s.AddLog(&ethgo.Log{}))
		stateSyncs, err := s.state.StateSyncStore.list()

		require.NoError(t, err)
		require.Len(t, stateSyncs, 0)

		var stateSyncedEvent contractsapi.StateSyncedEvent

		stateSyncEventID := stateSyncedEvent.Sig()

		// log with the state sync topic but incorrect content
		require.Error(t, s.AddLog(&ethgo.Log{Topics: []ethgo.Hash{stateSyncEventID}}))
		stateSyncs, err = s.state.StateSyncStore.list()

		require.NoError(t, err)
		require.Len(t, stateSyncs, 0)

		// correct event log
		data, err := abi.MustNewType("tuple(string a)").Encode([]string{"data"})
		require.NoError(t, err)

		goodLog := &ethgo.Log{
			Topics: []ethgo.Hash{
				stateSyncEventID,
				ethgo.BytesToHash([]byte{0x0}), // state sync index 0
				ethgo.ZeroHash,
				ethgo.ZeroHash,
			},
			Data: data,
		}

		require.NoError(t, s.AddLog(goodLog))

		stateSyncs, err = s.state.StateSyncStore.getStateSyncEventsForCommitment(0, 0, nil)
		require.NoError(t, err)
		require.Len(t, stateSyncs, 1)
		require.Len(t, s.pendingCommitments, 1)
		require.Equal(t, uint64(0), s.pendingCommitments[0].StartID.Uint64())
		require.Equal(t, uint64(0), s.pendingCommitments[0].EndID.Uint64())

		// add one more log to have a minimum commitment
		goodLog2 := goodLog.Copy()
		goodLog2.Topics[1] = ethgo.BytesToHash([]byte{0x1}) // state sync index 1
		require.NoError(t, s.AddLog(goodLog2))

		require.Len(t, s.pendingCommitments, 2)
		require.Equal(t, uint64(0), s.pendingCommitments[1].StartID.Uint64())
		require.Equal(t, uint64(1), s.pendingCommitments[1].EndID.Uint64())

		// add two more logs to have larger commitments
		goodLog3 := goodLog.Copy()
		goodLog3.Topics[1] = ethgo.BytesToHash([]byte{0x2}) // state sync index 2
		require.NoError(t, s.AddLog(goodLog3))

		goodLog4 := goodLog.Copy()
		goodLog4.Topics[1] = ethgo.BytesToHash([]byte{0x3}) // state sync index 3
		require.NoError(t, s.AddLog(goodLog4))

		require.Len(t, s.pendingCommitments, 4)
		require.Equal(t, uint64(0), s.pendingCommitments[3].StartID.Uint64())
		require.Equal(t, uint64(3), s.pendingCommitments[3].EndID.Uint64())
	})

	t.Run("Node is not a validator", func(t *testing.T) {
		t.Parallel()

		s := newTestStateSyncManager(t, vals.GetValidator("0"), &mockRuntime{isActiveValidator: false})

		// correct event log
		data, err := abi.MustNewType("tuple(string a)").Encode([]string{"data"})
		require.NoError(t, err)

		var stateSyncedEvent contractsapi.StateSyncedEvent

		goodLog := &ethgo.Log{
			Topics: []ethgo.Hash{
				stateSyncedEvent.Sig(),
				ethgo.BytesToHash([]byte{0x0}), // state sync index 0
				ethgo.ZeroHash,
				ethgo.ZeroHash,
			},
			Data: data,
		}

		require.NoError(t, s.AddLog(goodLog))

		// node should have inserted given state sync event, but it shouldn't build any commitment
		stateSyncs, err := s.state.StateSyncStore.getStateSyncEventsForCommitment(0, 0, nil)
		require.NoError(t, err)
		require.Len(t, stateSyncs, 1)
		require.Equal(t, uint64(0), stateSyncs[0].ID.Uint64())
		require.Len(t, s.pendingCommitments, 0)
	})
}

func TestStateSyncerManager_EventTracker_Sync(t *testing.T) {
	t.Parallel()

	vals := validator.NewTestValidators(t, 5)
	s := newTestStateSyncManager(t, vals.GetValidator("0"), &mockRuntime{isActiveValidator: true})

	server := testutil.DeployTestServer(t, nil)

	// Deploy contract
	contractReceipt, err := server.SendTxn(&ethgo.Transaction{
		Input: contractsapi.StateSender.Bytecode,
	})
	require.NoError(t, err)

	// Create contract function call payload
	encodedSyncStateData, err := (&contractsapi.SyncStateStateSenderFn{
		Receiver: types.BytesToAddress(server.Account(0).Bytes()),
		Data:     []byte{},
	}).EncodeAbi()
	require.NoError(t, err)

	// prefill with 10 events
	for i := 0; i < 10; i++ {
		receipt, err := server.SendTxn(&ethgo.Transaction{
			To:    &contractReceipt.ContractAddress,
			Input: encodedSyncStateData,
		})
		require.NoError(t, err)
		require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)
	}

	s.config.stateSenderAddr = types.Address(contractReceipt.ContractAddress)
	s.config.jsonrpcAddr = server.HTTPAddr()

	require.NoError(t, s.initTracker())

	time.Sleep(2 * time.Second)

	events, err := s.state.StateSyncStore.getStateSyncEventsForCommitment(1, 10, nil)
	require.NoError(t, err)
	require.Len(t, events, 10)
}

func TestStateSyncManager_Close(t *testing.T) {
	t.Parallel()

	mgr := newTestStateSyncManager(t, validator.NewTestValidator(t, "A", 100), &mockRuntime{isActiveValidator: true})
	require.NotPanics(t, func() { mgr.Close() })
}

func TestStateSyncManager_GetProofs(t *testing.T) {
	t.Parallel()

	const (
		stateSyncID = uint64(5)
	)

	state := newTestState(t)
	insertTestStateSyncProofs(t, state, maxCommitmentSize)

	stateSyncManager := &stateSyncManager{state: state}

	proof, err := stateSyncManager.GetStateSyncProof(stateSyncID)
	require.NoError(t, err)

	stateSync, ok := (proof.Metadata["StateSync"]).(*contractsapi.StateSyncedEvent)
	require.True(t, ok)
	require.Equal(t, stateSyncID, stateSync.ID.Uint64())
	require.NotEmpty(t, proof.Data)
}

func TestStateSyncManager_GetProofs_NoProof_NoCommitment(t *testing.T) {
	t.Parallel()

	const (
		stateSyncID = uint64(5)
	)

	stateSyncManager := &stateSyncManager{state: newTestState(t)}

	_, err := stateSyncManager.GetStateSyncProof(stateSyncID)
	require.ErrorContains(t, err, "cannot find commitment for StateSync id")
}

func TestStateSyncManager_GetProofs_NoProof_HasCommitment_NoStateSyncs(t *testing.T) {
	t.Parallel()

	const (
		stateSyncID = uint64(5)
	)

	state := newTestState(t)
	require.NoError(t, state.StateSyncStore.insertCommitmentMessage(createTestCommitmentMessage(t, 1), nil))

	stateSyncManager := &stateSyncManager{state: state, logger: hclog.NewNullLogger()}

	_, err := stateSyncManager.GetStateSyncProof(stateSyncID)
	require.ErrorContains(t, err, "failed to get state sync events for commitment to build proofs")
}

func TestStateSyncManager_GetProofs_NoProof_BuildProofs(t *testing.T) {
	t.Parallel()

	const (
		stateSyncID = uint64(5)
		fromIndex   = 1
	)

	state := newTestState(t)
	stateSyncs := generateStateSyncEvents(t, maxCommitmentSize, fromIndex)

	tree, err := createMerkleTree(stateSyncs)
	require.NoError(t, err)

	commitment := &CommitmentMessageSigned{
		Message: &contractsapi.StateSyncCommitment{
			StartID: big.NewInt(fromIndex),
			EndID:   big.NewInt(maxCommitmentSize),
			Root:    tree.Hash(),
		},
	}

	for _, sse := range stateSyncs {
		require.NoError(t, state.StateSyncStore.insertStateSyncEvent(sse))
	}

	require.NoError(t, state.StateSyncStore.insertCommitmentMessage(commitment, nil))

	stateSyncManager := &stateSyncManager{state: state, logger: hclog.NewNullLogger()}

	proof, err := stateSyncManager.GetStateSyncProof(stateSyncID)
	require.NoError(t, err)

	stateSync, ok := (proof.Metadata["StateSync"]).(*contractsapi.StateSyncedEvent)
	require.True(t, ok)
	require.Equal(t, stateSyncID, stateSync.ID.Uint64())
	require.NotEmpty(t, proof.Data)

	require.NoError(t, commitment.VerifyStateSyncProof(proof.Data, stateSync))
}

func createTestLogForStateSyncResultEvent(t *testing.T, stateSyncEventID uint64) *types.Log {
	t.Helper()

	var stateSyncResultEvent contractsapi.StateSyncResultEvent

	topics := make([]types.Hash, 3)
	topics[0] = types.Hash(stateSyncResultEvent.Sig())
	topics[1] = types.BytesToHash(common.EncodeUint64ToBytes(stateSyncEventID))
	topics[2] = types.BytesToHash(common.EncodeUint64ToBytes(1)) // Status = true
	someType := abi.MustNewType("tuple(string field1, string field2)")
	encodedData, err := someType.Encode(map[string]string{"field1": "value1", "field2": "value2"})
	require.NoError(t, err)

	return &types.Log{
		Address: contracts.StateReceiverContract,
		Topics:  topics,
		Data:    encodedData,
	}
}

type mockTopic struct {
	published proto.Message
}

func (m *mockTopic) consume() proto.Message {
	msg := m.published

	if m.published != nil {
		m.published = nil
	}

	return msg
}

func (m *mockTopic) Publish(obj proto.Message) error {
	m.published = obj

	return nil
}

func (m *mockTopic) Subscribe(handler func(obj interface{}, from peer.ID)) error {
	return nil
}

type mockMsg struct {
	hash  []byte
	epoch uint64
}

func newMockMsg() *mockMsg {
	hash := make([]byte, 32)
	rand.Read(hash)

	return &mockMsg{hash: hash}
}

func (m *mockMsg) WithHash(hash []byte) *mockMsg {
	m.hash = hash

	return m
}

func (m *mockMsg) sign(val *validator.TestValidator, domain []byte) (*TransportMessage, error) {
	signature, err := val.MustSign(m.hash, domain).Marshal()
	if err != nil {
		return nil, err
	}

	return &TransportMessage{
		Hash:        m.hash,
		Signature:   signature,
		From:        val.Address().String(),
		EpochNumber: m.epoch,
	}, nil
}

type mockRuntime struct {
	isActiveValidator bool
}

func (m *mockRuntime) IsActiveValidator() bool {
	return m.isActiveValidator
}
