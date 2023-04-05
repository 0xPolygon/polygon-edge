package polybft

import (
	"math/big"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/merkle-tree"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/testutil"
	"google.golang.org/protobuf/proto"
)

func newTestStateSyncManager(t *testing.T, key *testValidator) *stateSyncManager {
	t.Helper()

	tmpDir, err := os.MkdirTemp("/tmp", "test-data-dir-state-sync")
	require.NoError(t, err)

	state := newTestState(t)
	require.NoError(t, state.EpochStore.insertEpoch(0))

	topic := &mockTopic{}

	s, err := newStateSyncManager(hclog.NewNullLogger(), state,
		&stateSyncConfig{
			stateSenderAddr:   types.Address{},
			jsonrpcAddr:       "",
			dataDir:           tmpDir,
			topic:             topic,
			key:               key.Key(),
			maxCommitmentSize: maxCommitmentSize,
		})

	require.NoError(t, err)

	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	return s
}

func TestStateSyncManager_PostEpoch_BuildCommitment(t *testing.T) {
	vals := newTestValidators(t, 5)
	s := newTestStateSyncManager(t, vals.getValidator("0"))

	// there are no state syncs
	require.NoError(t, s.buildCommitment())
	require.Nil(t, s.pendingCommitments)

	stateSyncs10 := generateStateSyncEvents(t, 10, 0)

	// add 5 state syncs starting in index 0, it will generate one smaller commitment
	for i := 0; i < 5; i++ {
		require.NoError(t, s.state.StateSyncStore.insertStateSyncEvent(stateSyncs10[i]))
	}

	require.NoError(t, s.buildCommitment())
	require.Len(t, s.pendingCommitments, 1)
	require.Equal(t, uint64(0), s.pendingCommitments[0].StartID.Uint64())
	require.Equal(t, uint64(4), s.pendingCommitments[0].EndID.Uint64())
	require.Equal(t, uint64(0), s.pendingCommitments[0].Epoch)

	// add the next 5 state syncs, at that point, so that it generates a larger commitment
	for i := 5; i < 10; i++ {
		require.NoError(t, s.state.StateSyncStore.insertStateSyncEvent(stateSyncs10[i]))
	}

	require.NoError(t, s.buildCommitment())
	require.Len(t, s.pendingCommitments, 2)
	require.Equal(t, uint64(0), s.pendingCommitments[1].StartID.Uint64())
	require.Equal(t, uint64(9), s.pendingCommitments[1].EndID.Uint64())
	require.Equal(t, uint64(0), s.pendingCommitments[1].Epoch)

	// the message was sent
	require.NotNil(t, s.config.topic.(*mockTopic).consume()) //nolint
}

func TestStateSyncManager_MessagePool_OldEpoch(t *testing.T) {
	vals := newTestValidators(t, 5)

	s := newTestStateSyncManager(t, vals.getValidator("0"))
	s.epoch = 1

	msg := &TransportMessage{
		EpochNumber: 0,
	}
	err := s.saveVote(msg)
	require.NoError(t, err)
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

func (m *mockMsg) sign(val *testValidator, domain []byte) (*TransportMessage, error) {
	signature, err := val.mustSign(m.hash, domain).Marshal()
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

func TestStateSyncManager_MessagePool_SenderIsNoValidator(t *testing.T) {
	vals := newTestValidators(t, 5)

	s := newTestStateSyncManager(t, vals.getValidator("0"))
	s.validatorSet = vals.toValidatorSet()

	badVal := newTestValidator(t, "a", 0)
	msg, err := newMockMsg().sign(badVal, bls.DomainStateReceiver)
	require.NoError(t, err)

	require.Error(t, s.saveVote(msg))
}

func TestStateSyncManager_MessagePool_InvalidEpoch(t *testing.T) {
	t.Parallel()

	vals := newTestValidators(t, 5)

	s := newTestStateSyncManager(t, vals.getValidator("0"))
	s.validatorSet = vals.toValidatorSet()

	val := newMockMsg()
	msg, err := val.sign(vals.getValidator("0"), bls.DomainStateReceiver)
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
}

func TestStateSyncManager_MessagePool_SenderAndSignatureMissmatch(t *testing.T) {
	t.Parallel()

	vals := newTestValidators(t, 5)

	s := newTestStateSyncManager(t, vals.getValidator("0"))
	s.validatorSet = vals.toValidatorSet()

	// validator signs the msg in behalf of another validator
	val := newMockMsg()
	msg, err := val.sign(vals.getValidator("0"), bls.DomainStateReceiver)
	require.NoError(t, err)

	msg.From = vals.getValidator("1").Address().String()
	require.Error(t, s.saveVote(msg))

	// non validator signs the msg in behalf of a validator
	badVal := newTestValidator(t, "a", 0)
	msg, err = newMockMsg().sign(badVal, bls.DomainStateReceiver)
	require.NoError(t, err)

	msg.From = vals.getValidator("1").Address().String()
	require.Error(t, s.saveVote(msg))
}

func TestStateSyncManager_MessagePool_SenderVotes(t *testing.T) {
	vals := newTestValidators(t, 5)

	s := newTestStateSyncManager(t, vals.getValidator("0"))
	s.validatorSet = vals.toValidatorSet()

	msg := newMockMsg()
	val1signed, err := msg.sign(vals.getValidator("1"), bls.DomainStateReceiver)
	require.NoError(t, err)

	val2signed, err := msg.sign(vals.getValidator("2"), bls.DomainStateReceiver)
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
}

func TestStateSyncManager_BuildCommitment(t *testing.T) {
	vals := newTestValidators(t, 5)

	s := newTestStateSyncManager(t, vals.getValidator("0"))
	s.validatorSet = vals.toValidatorSet()

	// commitment is empty
	commitment, err := s.Commitment()
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
	signedMsg1, err := msg.sign(vals.getValidator("0"), bls.DomainStateReceiver)
	require.NoError(t, err)

	signedMsg2, err := msg.sign(vals.getValidator("1"), bls.DomainStateReceiver)
	require.NoError(t, err)

	require.NoError(t, s.saveVote(signedMsg1))
	require.NoError(t, s.saveVote(signedMsg2))

	commitment, err = s.Commitment()
	require.NoError(t, err) // there is no error if quorum is not met, since its a valid case
	require.Nil(t, commitment)

	// validator 2 and 3 vote for the proposal, there is enough voting power now

	signedMsg1, err = msg.sign(vals.getValidator("2"), bls.DomainStateReceiver)
	require.NoError(t, err)

	signedMsg2, err = msg.sign(vals.getValidator("3"), bls.DomainStateReceiver)
	require.NoError(t, err)

	require.NoError(t, s.saveVote(signedMsg1))
	require.NoError(t, s.saveVote(signedMsg2))

	commitment, err = s.Commitment()
	require.NoError(t, err)
	require.NotNil(t, commitment)
}

func TestStateSyncerManager_BuildProofs(t *testing.T) {
	vals := newTestValidators(t, 5)

	s := newTestStateSyncManager(t, vals.getValidator("0"))

	for _, evnt := range generateStateSyncEvents(t, 20, 0) {
		require.NoError(t, s.state.StateSyncStore.insertStateSyncEvent(evnt))
	}

	require.NoError(t, s.buildCommitment())
	require.Len(t, s.pendingCommitments, 1)

	mockMsg := &CommitmentMessageSigned{
		Message: &contractsapi.StateSyncCommitment{
			StartID: s.pendingCommitments[0].StartID,
			EndID:   s.pendingCommitments[0].EndID,
		},
	}

	txData, err := mockMsg.EncodeAbi()
	require.NoError(t, err)

	tx := createStateTransactionWithData(types.Address{}, txData)

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

func TestStateSyncerManager_AddLog_BuildCommitments(t *testing.T) {
	vals := newTestValidators(t, 5)

	s := newTestStateSyncManager(t, vals.getValidator("0"))

	// empty log which is not an state sync
	s.AddLog(&ethgo.Log{})
	stateSyncs, err := s.state.StateSyncStore.list()

	require.NoError(t, err)
	require.Len(t, stateSyncs, 0)

	var stateSyncedEvent contractsapi.StateSyncedEvent

	stateSyncEventID := stateSyncedEvent.Sig()

	// log with the state sync topic but incorrect content
	s.AddLog(&ethgo.Log{Topics: []ethgo.Hash{stateSyncEventID}})
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

	s.AddLog(goodLog)

	stateSyncs, err = s.state.StateSyncStore.getStateSyncEventsForCommitment(0, 0)
	require.NoError(t, err)
	require.Len(t, stateSyncs, 1)
	require.Len(t, s.pendingCommitments, 0)

	// add one more log to have a minimum commitment
	goodLog2 := goodLog.Copy()
	goodLog2.Topics[1] = ethgo.BytesToHash([]byte{0x1}) // state sync index 1
	s.AddLog(goodLog2)

	require.Len(t, s.pendingCommitments, 1)
	require.Equal(t, uint64(0), s.pendingCommitments[0].StartID.Uint64())
	require.Equal(t, uint64(1), s.pendingCommitments[0].EndID.Uint64())

	// add two more logs to have larger commitments
	goodLog3 := goodLog.Copy()
	goodLog3.Topics[1] = ethgo.BytesToHash([]byte{0x2}) // state sync index 2
	s.AddLog(goodLog3)

	goodLog4 := goodLog.Copy()
	goodLog4.Topics[1] = ethgo.BytesToHash([]byte{0x3}) // state sync index 3
	s.AddLog(goodLog4)

	require.Len(t, s.pendingCommitments, 3)
	require.Equal(t, uint64(0), s.pendingCommitments[2].StartID.Uint64())
	require.Equal(t, uint64(3), s.pendingCommitments[2].EndID.Uint64())
}

func TestStateSyncerManager_EventTracker_Sync(t *testing.T) {
	t.Parallel()

	vals := newTestValidators(t, 5)
	s := newTestStateSyncManager(t, vals.getValidator("0"))

	server := testutil.DeployTestServer(t, nil)

	//nolint:godox
	// TODO: Deploy local artifacts (to be fixed in EVM-542)
	cc := &testutil.Contract{}
	cc.AddCallback(func() string {
		return `
			event StateSynced(uint256 indexed id, address indexed sender, address indexed receiver, bytes data);
			uint256 indx;

			function emitEvent() public payable {
				emit StateSynced(indx, msg.sender, msg.sender, bytes(""));
				indx++;
			}
			`
	})

	_, addr, err := server.DeployContract(cc)
	require.NoError(t, err)

	// prefill with 10 events
	for i := 0; i < 10; i++ {
		receipt, err := server.TxnTo(addr, "emitEvent")
		require.NoError(t, err)
		require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)
	}

	s.config.stateSenderAddr = types.Address(addr)
	s.config.jsonrpcAddr = server.HTTPAddr()

	require.NoError(t, s.initTracker())

	time.Sleep(2 * time.Second)

	events, err := s.state.StateSyncStore.getStateSyncEventsForCommitment(0, 9)
	require.NoError(t, err)
	require.Len(t, events, 10)
}

func TestStateSyncManager_Close(t *testing.T) {
	t.Parallel()

	mgr := newTestStateSyncManager(t, newTestValidator(t, "A", 100))
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
	require.NoError(t, state.StateSyncStore.insertCommitmentMessage(createTestCommitmentMessage(t, 1)))

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

	require.NoError(t, state.StateSyncStore.insertCommitmentMessage(commitment))

	stateSyncManager := &stateSyncManager{state: state, logger: hclog.NewNullLogger()}

	proof, err := stateSyncManager.GetStateSyncProof(stateSyncID)
	require.NoError(t, err)

	stateSync, ok := (proof.Metadata["StateSync"]).(*contractsapi.StateSyncedEvent)
	require.True(t, ok)
	require.Equal(t, stateSyncID, stateSync.ID.Uint64())
	require.NotEmpty(t, proof.Data)

	require.NoError(t, commitment.VerifyStateSyncProof(proof.Data, stateSync))
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
