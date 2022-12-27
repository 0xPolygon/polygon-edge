package polybft

import (
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

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

	state := newTestState(t)
	require.NoError(t, state.insertEpoch(0))

	topic := &mockTopic{}

	s, err := NewStateSyncManager(hclog.NewNullLogger(), state,
		&stateSyncConfig{
			stateSenderAddr: types.Address{},
			jsonrpcAddr:     "",
			dataDir:         tmpDir,
			topic:           topic,
			key:             key.Key(),
		})

	require.NoError(t, err)

	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	return s
}

func TestStateSyncManager_PostEpoch_BuildCommitment(t *testing.T) {
	vals := newTestValidators(5)
	s := newTestStateSyncManager(t, vals.getValidator("0"))

	// there are no state syncs
	commitment, err := s.buildCommitment(0, 0)
	require.NoError(t, err)
	require.Nil(t, commitment)

	stateSyncs10 := generateStateSyncEvents(t, 20, 0)

	// add 5 state syncs starting in index 0, it will not generate a commitment
	for i := 0; i < 5; i++ {
		require.NoError(t, s.state.insertStateSyncEvent(stateSyncs10[i]))
	}

	commitment, err = s.buildCommitment(0, 0)
	require.NoError(t, err)
	require.Nil(t, commitment)

	// add the next 5 state syncs, at that point
	for i := 5; i < 10; i++ {
		require.NoError(t, s.state.insertStateSyncEvent(stateSyncs10[i]))
	}

	commitment, err = s.buildCommitment(0, 0)
	require.NoError(t, err)

	require.Equal(t, commitment.Epoch, uint64(0))
	require.Equal(t, commitment.FromIndex, uint64(0))
	require.Equal(t, commitment.ToIndex, uint64(9))

	// the message was sent
	require.NotNil(t, s.config.topic.(*mockTopic).consume()) //nolint
}

func TestStateSyncManager_MessagePool_OldEpoch(t *testing.T) {
	vals := newTestValidators(5)

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

func (m *mockMsg) sign(val *testValidator) *TransportMessage {
	signature, err := val.mustSign(m.hash).Marshal()
	if err != nil {
		panic(fmt.Errorf("BUG: %w", err))
	}

	return &TransportMessage{
		Hash:        m.hash,
		Signature:   signature,
		NodeID:      val.Address().String(),
		EpochNumber: m.epoch,
	}
}

func TestStateSyncManager_MessagePool_SenderIsNoValidator(t *testing.T) {
	vals := newTestValidators(5)

	s := newTestStateSyncManager(t, vals.getValidator("0"))
	s.validatorSet = vals.toValidatorSet()

	badVal := newTestValidator("a", 0)
	msg := newMockMsg().sign(badVal)

	err := s.saveVote(msg)
	require.Error(t, err)
}

func TestStateSyncManager_MessagePool_SenderVotes(t *testing.T) {
	vals := newTestValidators(5)

	s := newTestStateSyncManager(t, vals.getValidator("0"))
	s.validatorSet = vals.toValidatorSet()

	msg := newMockMsg()
	val1signed := msg.sign(vals.getValidator("1"))
	val2signed := msg.sign(vals.getValidator("2"))

	// vote with validator 1
	require.NoError(t, s.saveVote(val1signed))

	votes, err := s.state.getMessageVotes(0, msg.hash)
	require.NoError(t, err)
	require.Len(t, votes, 1)

	// vote with validator 1 again (the votes do not increase)
	require.NoError(t, s.saveVote(val1signed))
	votes, _ = s.state.getMessageVotes(0, msg.hash)
	require.Len(t, votes, 1)

	// vote with validator 2
	require.NoError(t, s.saveVote(val2signed))
	votes, _ = s.state.getMessageVotes(0, msg.hash)
	require.Len(t, votes, 2)
}

func TestStateSyncManager_BuildCommitment(t *testing.T) {
	vals := newTestValidators(5)

	s := newTestStateSyncManager(t, vals.getValidator("0"))
	s.validatorSet = vals.toValidatorSet()

	// commitment is empty
	commitment, err := s.Commitment()
	require.NoError(t, err)
	require.Nil(t, commitment)

	tree, err := NewMerkleTree([][]byte{{0x1}})
	require.NoError(t, err)

	s.commitment = &Commitment{
		MerkleTree: tree,
	}

	hash, err := s.commitment.Hash()
	require.NoError(t, err)

	msg := newMockMsg().WithHash(hash.Bytes())

	// validators 0 and 1 vote for the proposal, there is not enough
	// voting power for the proposal
	require.NoError(t, s.saveVote(msg.sign(vals.getValidator("0"))))
	require.NoError(t, s.saveVote(msg.sign(vals.getValidator("1"))))

	commitment, err = s.Commitment()
	require.NoError(t, err) // there is no error if quorum is not met, since its a valid case
	require.Nil(t, commitment)

	// validator 2 and 3 vote for the proposal, there is enough voting power now
	require.NoError(t, s.saveVote(msg.sign(vals.getValidator("2"))))
	require.NoError(t, s.saveVote(msg.sign(vals.getValidator("3"))))

	commitment, err = s.Commitment()
	require.NoError(t, err)
	require.NotNil(t, commitment)
}

func TestStateSyncerManager_BuildProofs(t *testing.T) {
	vals := newTestValidators(5)

	s := newTestStateSyncManager(t, vals.getValidator("0"))

	for _, evnt := range generateStateSyncEvents(t, 20, 0) {
		require.NoError(t, s.state.insertStateSyncEvent(evnt))
	}

	commitment, err := s.buildCommitment(0, 0)
	require.NoError(t, err)

	s.commitment = commitment

	mockMsg := &CommitmentMessageSigned{
		Message: &CommitmentMessage{
			FromIndex: commitment.FromIndex,
			ToIndex:   commitment.ToIndex,
		},
	}

	txData, err := mockMsg.EncodeAbi()
	require.NoError(t, err)

	tx := createStateTransactionWithData(types.Address{}, txData)

	req := &PostBlockRequest{
		Block: &types.Block{
			Transactions: []*types.Transaction{tx},
		},
	}
	require.NoError(t, s.PostBlock(req))

	for i := uint64(0); i < 10; i++ {
		proof, err := s.state.getStateSyncProof(i)
		require.NoError(t, err)
		require.NotNil(t, proof)
	}
}

func TestStateSyncerManager_EventTracker_AddLog(t *testing.T) {
	vals := newTestValidators(5)

	s := newTestStateSyncManager(t, vals.getValidator("0"))

	// empty log which is not an state sync
	s.AddLog(&ethgo.Log{})
	stateSyncs, err := s.state.list()

	require.NoError(t, err)
	require.Len(t, stateSyncs, 0)

	// log with the state sync topic but incorrect content
	s.AddLog(&ethgo.Log{Topics: []ethgo.Hash{stateTransferEventABI.ID()}})
	stateSyncs, err = s.state.list()

	require.NoError(t, err)
	require.Len(t, stateSyncs, 0)

	// correct event log
	data, err := abi.MustNewType("tuple(string a)").Encode([]string{"data"})
	require.NoError(t, err)

	goodLog := &ethgo.Log{
		Topics: []ethgo.Hash{
			stateTransferEventABI.ID(),
			ethgo.BytesToHash([]byte{0x1}), // state sync index 1
			ethgo.ZeroHash,
			ethgo.ZeroHash,
		},
		Data: data,
	}

	s.AddLog(goodLog)

	stateSyncs, err = s.state.getStateSyncEventsForCommitment(1, 1)
	require.NoError(t, err)
	require.Len(t, stateSyncs, 1)
}

func TestStateSyncerManager_EventTracker_Sync(t *testing.T) {
	t.Parallel()

	vals := newTestValidators(5)
	s := newTestStateSyncManager(t, vals.getValidator("0"))

	server := testutil.DeployTestServer(t, nil)

	// TODO: Deploy local artifacts
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

	events, err := s.state.getStateSyncEventsForCommitment(0, 9)
	require.NoError(t, err)
	require.Len(t, events, 10)
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
