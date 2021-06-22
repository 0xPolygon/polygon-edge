package ibft

import (
	"testing"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/consensus/ibft/proto"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/types"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

func TestTransition_ValidateState_Prepare(t *testing.T) {
	t.Skip()

	// we receive enough prepare messages to lock and commit the block
	i := newMockIbft(t, []string{"A", "B", "C", "D"}, "A")
	i.setState(ValidateState)

	i.emitMsg(&proto.MessageReq{
		From: "A",
		Type: proto.MessageReq_Prepare,
		View: proto.ViewMsg(1, 0),
	})
	i.emitMsg(&proto.MessageReq{
		From: "B",
		Type: proto.MessageReq_Prepare,
		View: proto.ViewMsg(1, 0),
	})
	// repeated message is not included
	i.emitMsg(&proto.MessageReq{
		From: "B",
		Type: proto.MessageReq_Prepare,
		View: proto.ViewMsg(1, 0),
	})
	i.emitMsg(&proto.MessageReq{
		From: "C",
		Type: proto.MessageReq_Prepare,
		View: proto.ViewMsg(1, 0),
	})
	i.Close()

	i.runCycle()

	i.expect(expectResult{
		sequence:    1,
		state:       ValidateState,
		prepareMsgs: 3,
		commitMsgs:  1, // A commit message
		locked:      true,
		outgoing:    1, // A commit message
	})
}

func TestTransition_ValidateState_CommitFastTrack(t *testing.T) {
	t.Skip()

	// we can directly receive the commit messages and fast track to the commit state
	// even when we do not have yet the preprepare messages
	i := newMockIbft(t, []string{"A", "B", "C", "D"}, "A")

	seal := hex.EncodeToHex(make([]byte, IstanbulExtraSeal))

	i.setState(ValidateState)
	i.state.view = proto.ViewMsg(1, 0)
	i.state.block = i.DummyBlock()
	i.state.locked = true

	i.emitMsg(&proto.MessageReq{
		From: "A",
		Type: proto.MessageReq_Commit,
		View: proto.ViewMsg(1, 0),
		Seal: seal,
	})
	i.emitMsg(&proto.MessageReq{
		From: "B",
		Type: proto.MessageReq_Commit,
		View: proto.ViewMsg(1, 0),
		Seal: seal,
	})
	i.emitMsg(&proto.MessageReq{
		From: "B",
		Type: proto.MessageReq_Commit,
		View: proto.ViewMsg(1, 0),
		Seal: seal,
	})
	i.emitMsg(&proto.MessageReq{
		From: "C",
		Type: proto.MessageReq_Commit,
		View: proto.ViewMsg(1, 0),
		Seal: seal,
	})

	i.runCycle()

	i.expect(expectResult{
		sequence:   1,
		commitMsgs: 3,
		outgoing:   1,
		locked:     false, // unlock after commit
	})
}

func TestTransition_AcceptState_ToSync(t *testing.T) {
	// we are in AcceptState and we are not in the validators list
	// means that we have been removed as validator, move to sync state
	i := newMockIbft(t, []string{"A", "B", "C", "D"}, "")
	i.setState(AcceptState)
	i.Close()

	// we are the proposer and we need to build a block
	i.runCycle()

	i.expect(expectResult{
		sequence: 1,
		state:    SyncState,
	})
}

func TestTransition_AcceptState_Proposer_Locked(t *testing.T) {
	// If we are the proposer and there is a lock value we need to propose it
	i := newMockIbft(t, []string{"A", "B", "C", "D"}, "A")
	i.setState(AcceptState)

	i.state.locked = true
	i.state.block = &types.Block{
		Header: &types.Header{
			Number: 10,
		},
	}

	i.runCycle()

	i.expect(expectResult{
		sequence: 1,
		state:    ValidateState,
		locked:   true,
		outgoing: 2, // preprepare and prepare
	})
	if i.state.block.Number() != 10 {
		t.Fatal("bad block")
	}
}

func TestTransition_AcceptState_Validator_VerifyCorrect(t *testing.T) {
	i := newMockIbft(t, []string{"A", "B", "C"}, "B")
	i.state.view = proto.ViewMsg(1, 0)
	i.setState(AcceptState)

	block := i.DummyBlock()
	header, err := writeSeal(i.pool.get("A").priv, block.Header)
	assert.NoError(t, err)
	block.Header = header

	// A sends the message
	i.emitMsg(&proto.MessageReq{
		From: "A",
		Type: proto.MessageReq_Preprepare,
		Proposal: &any.Any{
			Value: block.MarshalRLP(),
		},
		View: proto.ViewMsg(1, 0),
	})

	i.runCycle()

	i.expect(expectResult{
		sequence: 1,
		state:    ValidateState,
		outgoing: 1, // prepare
	})
}

func TestTransition_AcceptState_Validator_VerifyFails(t *testing.T) {
	i := newMockIbft(t, []string{"A", "B", "C"}, "B")
	i.state.view = proto.ViewMsg(1, 0)
	i.setState(AcceptState)

	block := i.DummyBlock()
	block.Header.MixHash = types.Hash{} // invalidates the block

	header, err := writeSeal(i.pool.get("A").priv, block.Header)
	assert.NoError(t, err)
	block.Header = header

	// A sends the message
	i.emitMsg(&proto.MessageReq{
		From: "A",
		Type: proto.MessageReq_Preprepare,
		Proposal: &any.Any{
			Value: block.MarshalRLP(),
		},
		View: proto.ViewMsg(1, 0),
	})

	i.runCycle()

	i.expect(expectResult{
		sequence: 1,
		state:    RoundChangeState,
		err:      errBlockVerificationFailed,
	})
}

func TestTransition_AcceptState_Validator_ProposerInvalid(t *testing.T) {
	i := newMockIbft(t, []string{"A", "B", "C"}, "B")
	i.state.view = proto.ViewMsg(1, 0)
	i.setState(AcceptState)

	// A is the proposer but C sends the propose, we do not fail
	// but wait for timeout to move to roundChange state
	i.emitMsg(&proto.MessageReq{
		From: "C",
		Type: proto.MessageReq_Preprepare,
		Proposal: &any.Any{
			Value: i.DummyBlock().MarshalRLP(),
		},
		View: proto.ViewMsg(1, 0),
	})
	i.forceTimeout()

	i.runCycle()

	i.expect(expectResult{
		sequence: 1,
		state:    RoundChangeState,
	})
}

func TestTransition_AcceptState_Validator_LockWrong(t *testing.T) {
	i := newMockIbft(t, []string{"A", "B", "C"}, "B")
	i.state.view = proto.ViewMsg(1, 0)
	i.setState(AcceptState)

	// locked block
	block := i.DummyBlock()
	block.Header.Number = 1
	block.Header.ComputeHash()

	i.state.block = block
	i.state.locked = true

	// proposed block
	block1 := i.DummyBlock()
	block1.Header.Number = 2
	block1.Header.ComputeHash()

	i.emitMsg(&proto.MessageReq{
		From: "A",
		Type: proto.MessageReq_Preprepare,
		Proposal: &any.Any{
			Value: block1.MarshalRLP(),
		},
		View: proto.ViewMsg(1, 0),
	})

	i.runCycle()

	i.expect(expectResult{
		sequence: 1,
		state:    RoundChangeState,
		locked:   true,
		err:      errIncorrectBlockLocked,
	})
}

func TestTransition_AcceptState_Validator_LockCorrect(t *testing.T) {
	i := newMockIbft(t, []string{"A", "B", "C"}, "B")
	i.state.view = proto.ViewMsg(1, 0)
	i.setState(AcceptState)

	// locked block
	block := i.DummyBlock()
	block.Header.Number = 1
	block.Header.ComputeHash()

	i.state.block = block
	i.state.locked = true

	i.emitMsg(&proto.MessageReq{
		From: "A",
		Type: proto.MessageReq_Preprepare,
		Proposal: &any.Any{
			Value: block.MarshalRLP(),
		},
		View: proto.ViewMsg(1, 0),
	})

	i.runCycle()

	i.expect(expectResult{
		sequence: 1,
		state:    ValidateState,
		locked:   true,
		outgoing: 1, // prepare message
	})
}

func TestTransition_RoundChangeState_CatchupRound(t *testing.T) {
	m := newMockIbft(t, []string{"A", "B", "C", "D"}, "A")
	m.setState(RoundChangeState)

	// new messages arrive with round number 2
	m.emitMsg(&proto.MessageReq{
		From: "B",
		Type: proto.MessageReq_RoundChange,
		View: proto.ViewMsg(1, 2),
	})
	m.emitMsg(&proto.MessageReq{
		From: "C",
		Type: proto.MessageReq_RoundChange,
		View: proto.ViewMsg(1, 2),
	})
	m.emitMsg(&proto.MessageReq{
		From: "D",
		Type: proto.MessageReq_RoundChange,
		View: proto.ViewMsg(1, 2),
	})
	m.Close()

	// as soon as it starts it will move to round 1 because it has
	// not processed all the messages yet.
	// After it receives 3 Round change messages higher than his own
	// round it will change round again and move to accept
	m.runCycle()

	m.expect(expectResult{
		sequence: 1,
		round:    2,
		outgoing: 1, // our new round change
		state:    AcceptState,
	})
}

func TestTransition_RoundChangeState_Timeout(t *testing.T) {
	m := newMockIbft(t, []string{"A", "B", "C", "D"}, "A")

	m.forceTimeout()
	m.setState(RoundChangeState)
	m.Close()

	// increases to round 1 at the beginning of the round and sends
	// one RoundChange message.
	// After the timeout, it increases to round 2 and sends another
	/// RoundChange message.
	m.runCycle()

	m.expect(expectResult{
		sequence: 1,
		round:    2,
		outgoing: 2, // two round change messages
		state:    RoundChangeState,
	})
}

func TestTransition_RoundChangeState_WeakCertificate(t *testing.T) {
	m := newMockIbft(t, []string{"A", "B", "C", "D", "E", "F", "G"}, "A")

	m.setState(RoundChangeState)

	// send three roundChange messages which are enough to force a
	// weak change where the client moves to that new round state
	m.emitMsg(&proto.MessageReq{
		From: "B",
		Type: proto.MessageReq_RoundChange,
		View: proto.ViewMsg(1, 2),
	})
	m.emitMsg(&proto.MessageReq{
		From: "C",
		Type: proto.MessageReq_RoundChange,
		View: proto.ViewMsg(1, 2),
	})
	m.emitMsg(&proto.MessageReq{
		From: "D",
		Type: proto.MessageReq_RoundChange,
		View: proto.ViewMsg(1, 2),
	})
	m.Close()

	m.runCycle()

	m.expect(expectResult{
		sequence: 1,
		round:    2,
		outgoing: 2, // two round change messages (0->1, 1->2 after weak certificate)
		state:    RoundChangeState,
	})
}

func TestTransition_RoundChangeState_ErrStartNewRound(t *testing.T) {
	// if we start a round change because there was an error we start
	// a new round right away
	m := newMockIbft(t, []string{"A", "B"}, "A")
	m.Close()

	m.state.err = errBlockVerificationFailed

	m.setState(RoundChangeState)
	m.runCycle()

	m.expect(expectResult{
		sequence: 1,
		round:    1,
		state:    RoundChangeState,
		outgoing: 1,
	})
}

func TestTransition_RoundChangeState_StartNewRound(t *testing.T) {
	// if we start round change due to a state timeout and we are on the
	// correct sequence, we start a new round
	m := newMockIbft(t, []string{"A", "B"}, "A")
	m.Close()

	m.state.view.Sequence = 1

	m.setState(RoundChangeState)
	m.runCycle()

	m.expect(expectResult{
		sequence: 1,
		round:    1,
		state:    RoundChangeState,
		outgoing: 1,
	})
}

func TestTransition_RoundChangeState_MaxRound(t *testing.T) {
	// if we start round change due to a state timeout we try to catch up
	// with the highest round seen.
	m := newMockIbft(t, []string{"A", "B", "C"}, "A")
	m.Close()

	m.addMessage(&proto.MessageReq{
		From: "B",
		Type: proto.MessageReq_RoundChange,
		View: &proto.View{
			Round:    10,
			Sequence: 1,
		},
	})

	m.setState(RoundChangeState)
	m.runCycle()

	m.expect(expectResult{
		sequence: 1,
		round:    10,
		state:    RoundChangeState,
		outgoing: 1,
	})
}

type mockIbft struct {
	t *testing.T
	*Ibft

	blockchain *blockchain.Blockchain
	pool       *testerAccountPool
	respMsg    []*proto.MessageReq
}

func (m *mockIbft) DummyBlock() *types.Block {
	parent, _ := m.blockchain.GetHeaderByNumber(0)
	block := &types.Block{
		Header: &types.Header{
			ExtraData:  parent.ExtraData,
			MixHash:    IstanbulDigest,
			Sha3Uncles: types.EmptyUncleHash,
		},
	}
	return block
}

func (m *mockIbft) Header() *types.Header {
	return m.blockchain.Header()
}

func (m *mockIbft) GetHeaderByNumber(i uint64) (*types.Header, bool) {
	return m.blockchain.GetHeaderByNumber(i)
}

func (m *mockIbft) WriteBlocks(blocks []*types.Block) error {
	return nil
}

func (m *mockIbft) emitMsg(msg *proto.MessageReq) {
	// convert the address from the address pool
	from := m.pool.get(msg.From).Address()
	msg.From = from.String()

	m.Ibft.pushMessage(msg)
}

func (m *mockIbft) addMessage(msg *proto.MessageReq) {
	// convert the address from the address pool
	from := m.pool.get(msg.From).Address()
	msg.From = from.String()

	m.state.addMessage(msg)
}

func (m *mockIbft) Gossip(msg *proto.MessageReq) error {
	m.respMsg = append(m.respMsg, msg)
	return nil
}

func newMockIbft(t *testing.T, accounts []string, account string) *mockIbft {
	pool := newTesterAccountPool()
	pool.add(accounts...)

	m := &mockIbft{
		t:          t,
		pool:       pool,
		blockchain: blockchain.TestBlockchain(t, pool.genesis()),
		respMsg:    []*proto.MessageReq{},
	}

	var addr *testerAccount
	if account == "" {
		// account not in validator set, create a new one that is not part
		// of the genesis
		pool.add("xx")
		addr = pool.get("xx")
	} else {
		addr = pool.get(account)
	}
	ibft := &Ibft{
		logger:           hclog.NewNullLogger(),
		config:           &consensus.Config{},
		blockchain:       m,
		validatorKey:     addr.priv,
		validatorKeyAddr: addr.Address(),
		closeCh:          make(chan struct{}),
		updateCh:         make(chan struct{}),
		operator:         &operator{},
		state:            newState(),
		epochSize:        DefaultEpochSize,
	}

	// by default set the state to (1, 0)
	ibft.state.view = proto.ViewMsg(1, 0)

	m.Ibft = ibft

	assert.NoError(t, ibft.setupSnapshot())
	assert.NoError(t, ibft.createKey())

	// set the initial validators frrom the snapshot
	ibft.state.validators = pool.ValidatorSet()

	m.Ibft.transport = m
	return m
}

type expectResult struct {
	state    IbftState
	sequence uint64
	round    uint64
	locked   bool
	err      error

	// num of messages
	prepareMsgs uint64
	commitMsgs  uint64

	// outgoing messages
	outgoing uint64
}

func (m *mockIbft) expect(res expectResult) {
	if sequence := m.state.view.Sequence; sequence != res.sequence {
		m.t.Fatalf("incorrect sequence %d %d", sequence, res.sequence)
	}
	if round := m.state.view.Round; round != res.round {
		m.t.Fatalf("incorrect round %d %d", round, res.round)
	}
	if m.getState() != res.state {
		m.t.Fatalf("incorrect state %s %s", m.getState(), res.state)
	}
	if size := len(m.state.prepared); uint64(size) != res.prepareMsgs {
		m.t.Fatalf("incorrect prepared messages %d %d", size, res.prepareMsgs)
	}
	if size := len(m.state.committed); uint64(size) != res.commitMsgs {
		m.t.Fatalf("incorrect commit messages %d %d", size, res.commitMsgs)
	}
	if m.state.locked != res.locked {
		m.t.Fatalf("incorrect locked %v %v", m.state.locked, res.locked)
	}
	if size := len(m.respMsg); uint64(size) != res.outgoing {
		m.t.Fatalf("incorrect outgoing messages %v %v", size, res.outgoing)
	}
	if m.state.err != res.err {
		m.t.Fatalf("incorrect error %v %v", m.state.err, res.err)
	}
}
