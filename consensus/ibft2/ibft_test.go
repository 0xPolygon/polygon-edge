package ibft2

import (
	"strconv"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/consensus/ibft2/proto"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/types"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

func makeServers(t *testing.T, num int) []*Ibft2 {
	transport := &mockNetwork{}
	pool := newTesterAccountPool(num)

	servers := []*Ibft2{}
	for i := 0; i < num; i++ {
		blockchain := blockchain.TestBlockchain(t, pool.genesis())

		logger := hclog.New(&hclog.LoggerOptions{
			Level: hclog.LevelFromString("debug"),
		})

		acct := pool.get(strconv.Itoa(i))
		p := &Ibft2{
			logger:           logger,
			blockchain:       blockchain,
			validatorKey:     acct.priv,
			validatorKeyAddr: acct.Address(),
			transportFactory: transport.newTransport,
		}
		p.createKey()
		servers = append(servers, p)
	}

	// start the seal
	for _, srv := range servers {
		srv.StartSeal()
	}
	return servers
}

func TestStatxs(t *testing.T) {
	makeServers(t, 3)

	time.Sleep(5 * time.Second)
}

func TestCmpView(t *testing.T) {
	var cases = []struct {
		v, y *proto.View
		res  int
	}{
		{
			&proto.View{
				Sequence: 1,
				Round:    1,
			},
			&proto.View{
				Sequence: 2,
				Round:    1,
			},
			-1,
		},
	}

	for _, c := range cases {
		assert.Equal(t, cmpView(c.v, c.y), c.res)
	}
}

func TestTransition_ValidateState(t *testing.T) {
	t.Run("Prepare", func(t *testing.T) {
		// we receive enough prepare messages to lock and commit the block
		i := newMockIbft2(t, []string{"A", "B", "C", "D"}, "A")

		i.setState(ValidateState)
		i.state2.view = proto.ViewMsg(1, 0)

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
		i.stop()

		i.runCycle()

		i.expect(expectResult{
			sequence:    1,
			state:       ValidateState,
			prepareMsgs: 3,
			commitMsgs:  1, // A commit message
			locked:      true,
			outgoing:    1, // A commit message
		})
	})

	t.Run("CommitFastTrack", func(t *testing.T) {
		// we can directly receive the commit messages and fast track to the commit state
		// even when we do not have yet the preprepare messages
		i := newMockIbft2(t, []string{"A", "B", "C", "D"}, "A")

		seal := hex.EncodeToHex(make([]byte, IstanbulExtraSeal))

		i.setState(ValidateState)
		i.state2.view = proto.ViewMsg(1, 0)
		i.state2.block = i.DummyBlock()
		i.state2.locked = true

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
	})
}

func TestTransition_AcceptState_Proposer(t *testing.T) {
	setup := func() *mockIbft2 {
		i := newMockIbft2(t, []string{"A", "B", "C", "D"}, "A")
		i.setState(AcceptState)
		return i
	}

	t.Run("Valid proposer", func(t *testing.T) {
		// we are the proposer and we need to build a block
		i := setup()
		i.runCycle()

		i.expect(expectResult{
			state:    ValidateState,
			outgoing: 2, // preprepare and prepare
		})
	})

	t.Run("Locked", func(t *testing.T) {
		// If we are the proposer and there is a lock value we need to propose it
		i := setup()
		i.state2.locked = true
		i.state2.block = &types.Block{
			Header: &types.Header{
				Number: 10,
			},
		}

		i.runCycle()

		i.expect(expectResult{
			state:    ValidateState,
			locked:   true,
			outgoing: 2, // preprepare and prepare
		})
		if i.state2.block.Number() != 10 {
			t.Fatal("bad block")
		}
	})
}

func TestTransition_AcceptState_Validator(t *testing.T) {
	setup := func() *mockIbft2 {
		i := newMockIbft2(t, []string{"A", "B", "C"}, "B")
		i.state2.view = proto.ViewMsg(1, 0)
		i.setState(AcceptState)
		return i
	}

	t.Run("Verify Correct", func(t *testing.T) {
		i := setup()

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
	})

	t.Run("Verify Fails", func(t *testing.T) {
		i := setup()

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
	})

	// TODO: Fix timeouts in testing
	/*
		t.Run("Proposer Invalid", func(t *testing.T) {
			i := setup()

			// A is the proposer but C sends the propose
			i.emitMsg(&mockMsg{
				From:    "C",
				MsgType: msgPreprepare,
				Block:   i.DummyBlock(),
				View:    proto.ViewMsg(1, 0),
			})

			i.runCycle()

			i.expect(expectResult{
				sequence: 1,
				state:    RoundChangeState,
			})
		})
	*/

	t.Run("Lock Wrong", func(t *testing.T) {
		i := setup()

		// locked block
		block := i.DummyBlock()
		block.Header.Number = 1
		block.Header.ComputeHash()

		i.state2.block = block
		i.state2.locked = true

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
	})

	t.Run("Lock Correct", func(t *testing.T) {
		i := setup()

		// locked block
		block := i.DummyBlock()
		block.Header.Number = 1
		block.Header.ComputeHash()

		i.state2.block = block
		i.state2.locked = true

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
	})
}

func TestTransition_ChangeRound(t *testing.T) {
	m := newMockIbft2(t, []string{"A", "B", "C", "D"}, "A")
	m.Close()

	m.state2.view.Sequence = 1
	m.state2.view.Round = 0 // it will start in round 1

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

	m.setState(RoundChangeState)
	m.runCycle()

	m.expect(expectResult{
		sequence: 1,
		round:    2,
		outgoing: 1, // our new round change
	})
}

func TestTransition_RoundChangeState(t *testing.T) {
	t.Run("Init ErrStartNewRound", func(t *testing.T) {
		// if we start a round change because there was an error we start
		// a new round right away
		m := newMockIbft2(t, []string{"A", "B"}, "A")
		m.Close()

		m.state2.err = errBlockVerificationFailed

		m.setState(RoundChangeState)
		m.runCycle()

		m.expect(expectResult{
			sequence: 0,
			round:    1,
			state:    RoundChangeState,
			outgoing: 1,
		})
	})

	t.Run("Init CatchupProposal", func(t *testing.T) {
		// if we start round change due to a state timeout and we are NOT on
		// the correct sequence we try to catch up with the sequence
		m := newMockIbft2(t, []string{"A", "B"}, "A")

		m.setState(RoundChangeState)
		m.runCycle()

		m.expect(expectResult{
			sequence: 1,
			round:    0, // ??
			state:    AcceptState,
		})
	})

	t.Run("Init StartNewRound", func(t *testing.T) {
		// if we start round change due to a state timeout and we are on the
		// correct sequence, we start a new round
		m := newMockIbft2(t, []string{"A", "B"}, "A")
		m.Close()

		m.state2.view.Sequence = 1

		m.setState(RoundChangeState)
		m.runCycle()

		m.expect(expectResult{
			sequence: 1,
			round:    1,
			state:    RoundChangeState,
			outgoing: 1,
		})
	})

	/*
		t.Run("Init MaxRound", func(t *testing.T) {
			// if we start round change due to a state timeout we try to catch up
			// with the highest round seen.
			m := newMockIbft2(t, []string{"A", "B", "C"}, "A")
			m.Close()

			m.state2.view.Sequence = 1
			m.state2.AddRoundMessage(proto.RoundChange(m.pool.get("B").Address(), &proto.Subject{
				View: &proto.View{
					Round:    10,
					Sequence: 1,
				},
			}))

			m.setState(RoundChangeState)
			m.runCycle()

			m.expect(expectResult{
				sequence: 1,
				round:    10,
				state:    RoundChangeState,
				outgoing: 1,
			})
		})
	*/

	t.Run("Timeout", func(t *testing.T) {
		// there is a timeout on the round change state, it starts a new
		// round with a higher number
	})

	t.Run("WeakCertificate", func(t *testing.T) {
		// there are MinFaultyNodes()+1 of a given round, try to catch up with
		// that round if is lower than our current round and reset the timer.
	})

	t.Run("Completed", func(t *testing.T) {
		// more than 2*i.state2.MinFaultyNodes()+1 round changes have arrived for a
		// specific round, we need to accept proposals for the new round.
	})
}

func TestTransition_RoundChange_ToSync(t *testing.T) {
	// check that if there are some issues we can move
}

func TestTransition_Sync_ToAccep(t *testing.T) {
	// check that after we do sync we move to accept state
}

type mockMsg struct {
	From    string
	MsgType MsgType
	View    *proto.View
	Seal    []byte
	Block   *types.Block
}

type mockIbft2 struct {
	t *testing.T
	*Ibft2

	blockchain *blockchain.Blockchain
	pool       *testerAccountPool
	// ch         chan *proto.MessageReq
	respMsg []*proto.MessageReq
}

func (m *mockIbft2) DummyBlock() *types.Block {
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

func (m *mockIbft2) Header() *types.Header {
	return m.blockchain.Header()
}

func (m *mockIbft2) GetHeaderByNumber(i uint64) (*types.Header, bool) {
	return m.blockchain.GetHeaderByNumber(i)
}

func (m *mockIbft2) WriteBlocks(blocks []*types.Block) error {
	return nil
}

func (m *mockIbft2) forceTimeout() {

}

func (m *mockIbft2) stop() {
	m.Ibft2.Close()
}

func (m *mockIbft2) emitMsg(msg *proto.MessageReq) {
	// convert the address from the address pool
	from := m.pool.get(msg.From).Address()
	msg.From = from.String()

	m.Ibft2.pushMessage(msg)
}

func (m *mockIbft2) Gossip(msg *proto.MessageReq) error {
	m.respMsg = append(m.respMsg, msg)
	return nil
}

/*
func (m *mockIbft2) Listen() chan *proto.MessageReq {
	return m.ch
}
*/

func newMockIbft2(t *testing.T, accounts []string, account string) *mockIbft2 {
	pool := newTesterAccountPool()
	pool.add(accounts...)

	m := &mockIbft2{
		t:          t,
		pool:       pool,
		blockchain: blockchain.TestBlockchain(t, pool.genesis()),
		// ch:         make(chan *proto.MessageReq, 1),
		respMsg: []*proto.MessageReq{},
	}

	addr := pool.get(account)
	ibft := &Ibft2{
		logger:           hclog.NewNullLogger(),
		blockchain:       m,
		validatorKey:     addr.priv,
		validatorKeyAddr: addr.Address(),
		closeCh:          make(chan struct{}),
		updateCh:         make(chan struct{}),
		maxTimeoutRange:  1 * time.Second,
		operator:         &operator{},
		state2:           newState(),
	}
	m.Ibft2 = ibft

	assert.NoError(t, ibft.setupSnapshot())
	assert.NoError(t, ibft.createKey())

	// set the initial validators frrom the snapshot
	ibft.state2.validators = pool.ValidatorSet()

	m.Ibft2.transport = m
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

func (m *mockIbft2) expect(res expectResult) {
	if sequence := m.state2.view.Sequence; sequence != res.sequence {
		m.t.Fatalf("incorrect sequence %d %d", sequence, res.sequence)
	}
	if round := m.state2.view.Round; round != res.round {
		m.t.Fatalf("incorrect sequence %d %d", round, res.round)
	}
	if m.getState() != res.state {
		m.t.Fatalf("incorrect state %s %s", m.getState(), res.state)
	}
	if size := len(m.state2.prepared); uint64(size) != res.prepareMsgs {
		m.t.Fatalf("incorrect prepared messages %d %d", size, res.prepareMsgs)
	}
	if size := len(m.state2.committed); uint64(size) != res.commitMsgs {
		m.t.Fatalf("incorrect commit messages %d %d", size, res.commitMsgs)
	}
	if m.state2.locked != res.locked {
		m.t.Fatalf("incorrect locked %v %v", m.state2.locked, res.locked)
	}
	if size := len(m.respMsg); uint64(size) != res.outgoing {
		m.t.Fatalf("incorrect outgoing messages %v %v", size, res.outgoing)
	}
	if m.state2.err != res.err {
		m.t.Fatalf("incorrect error %v %v", m.state2.err, res.err)
	}
}
