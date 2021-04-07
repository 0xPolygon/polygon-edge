package ibft2

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/consensus/ibft2/proto"
	"github.com/0xPolygon/minimal/types"
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

func TestStates(t *testing.T) {
	makeServers(t, 3)

	time.Sleep(5 * time.Second)
}

func TestNextMessage(t *testing.T) {
	i := &Ibft2{
		msgQueue: msgQueueImpl{},
		updateCh: make(chan struct{}),
		state2: &currentState{
			view: &proto.View{
				Round:    1,
				Sequence: 1,
			},
		},
	}

	i.pushMessage(&proto.MessageReq{
		Message: &proto.MessageReq_Prepare{
			Prepare: i.state2.Subject(),
		},
	})

	i.getNextMessage(nil)

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

func TestStateAddMessageCommit(t *testing.T) {
	pool := newTesterAccountPool(5)
	state := &currentState{
		validators: pool.ValidatorSet(),
	}

	// we can use the same dummy subject since currentState DOES NOT
	// validate sequence and round. That is filtered during the message queue
	subject := &proto.Subject{
		View: &proto.View{
			Sequence: 0,
			Round:    0,
		},
	}

	state.addCommited(proto.Commit(pool.indx(1).Address(), subject))
	assert.Equal(t, state.numCommited(), 1)

	// including the same item twice does not increase numCommitted
	state.addCommited(proto.Commit(pool.indx(1).Address(), subject))
	assert.Equal(t, state.numCommited(), 1)

	// you cannot include an item with an incorrect addr
	state.addCommited(proto.Commit(types.ZeroAddress, subject))
	assert.Equal(t, state.numCommited(), 1)

	state.addCommited(proto.Commit(pool.indx(2).Address(), subject))
	assert.Equal(t, state.numCommited(), 2)
}

func TestStateAddMessageRound(t *testing.T) {
	pool := newTesterAccountPool(5)
	state := &currentState{
		validators: pool.ValidatorSet(),
	}

	getRound := func(round uint64) *proto.Subject {
		return &proto.Subject{
			View: &proto.View{
				Round:    round,
				Sequence: 0,
			},
		}
	}

	num := state.AddRoundMessage(proto.RoundChange(pool.indx(1).Address(), getRound(1)))
	assert.Equal(t, num, 1)

	// cannot add the same message twice
	num = state.AddRoundMessage(proto.RoundChange(pool.indx(1).Address(), getRound(1)))
	assert.Equal(t, num, 1)

	// add message in another round
	state.AddRoundMessage(proto.RoundChange(pool.indx(2).Address(), getRound(2)))
	assert.Equal(t, state.numRounds(1), 1)
	assert.Equal(t, state.numRounds(2), 1)
}

/*
func TestTransitionProposal(t *testing.T) {

}
*/

type mockTranport2 struct {
	ch   chan *proto.MessageReq
	msgs []*proto.MessageReq
}

func (m *mockTranport2) Gossip(target []types.Address, msg *proto.MessageReq) error {
	if len(m.msgs) == 0 {
		m.msgs = make([]*proto.MessageReq, 0)
	}
	fmt.Println("_XXX")
	m.msgs = append(m.msgs, msg)
	return nil
}

func (m *mockTranport2) Listen() chan *proto.MessageReq {
	return m.ch
}

func mockIbft(addr types.Address, pool *testerAccountPool, t *testing.T, msgs []*proto.MessageReq) *Ibft2 {
	i := &Ibft2{
		logger:           hclog.NewNullLogger(),
		blockchain:       blockchain.TestBlockchain(t, pool.genesis()),
		validatorKeyAddr: addr,
		closeCh:          make(chan struct{}),
		updateCh:         make(chan struct{}),
		maxTimeoutRange:  1 * time.Second,
	}
	assert.NoError(t, i.setupSnapshot())
	ch := make(chan *proto.MessageReq)
	go func() {
		for _, msg := range msgs {
			ch <- msg
		}
		// stop the client when there are no more messages
		i.Close()
	}()
	i.transport = &mockTranport2{
		ch: ch,
	}
	go i.readMessages()
	return i
}

func TestTransition_AcceptState_ProposeBlock(t *testing.T) {
	pool := newTesterAccountPool(5)

	i := mockIbft(pool.indx(0).Address(), pool, t, nil)
	i.runAcceptState()

	// transition to AcceptState
	assert.True(t, i.isState(ValidateState))
	// TODO: Sent a preprepare message
}

func TestTransition_AcceptState_ProposeLockedBlock(t *testing.T) {

}

func TestTransition_AcceptState_Validator_WrongProposer(t *testing.T) {
	pool := newTesterAccountPool(5)

	msgs := []*proto.MessageReq{
		proto.PreprepareMsg(pool.indx(2).Address(), &proto.Preprepare{
			View: &proto.View{
				Round:    0,
				Sequence: 1,
			},
			Proposal: &proto.Proposal{},
		}),
	}
	i := mockIbft(pool.indx(1).Address(), pool, t, msgs)
	i.runAcceptState()

	// the proposal was made from the wrong proposer
	assert.True(t, i.isState(RoundChangeState))
}

func TestTransition_ValidateState_Timeout(t *testing.T) {
	pool := newTesterAccountPool(5)

	i := mockIbft(pool.indx(1).Address(), pool, t, nil)
	i.setState(ValidateState)
	i.runValidateState()

	assert.True(t, i.isState(RoundChangeState))
}

func TestTransition_ValidateState_ReceivePrepare(t *testing.T) {
	// we receive enough prepare messages to lock and commit the block
	pool := newTesterAccountPool(5)

	msgs := []*proto.MessageReq{
		proto.Prepare(pool.indx(1).Address(), &proto.Subject{
			View: proto.ViewMsg(1, 0),
		}),
		proto.Prepare(pool.indx(2).Address(), &proto.Subject{
			View: proto.ViewMsg(1, 0),
		}),
		proto.Prepare(pool.indx(2).Address(), &proto.Subject{
			View: proto.ViewMsg(1, 0),
		}),
		proto.Prepare(pool.indx(3).Address(), &proto.Subject{
			View: proto.ViewMsg(1, 0),
		}),
	}
	i := mockIbft(pool.indx(0).Address(), pool, t, msgs)
	i.state2 = &currentState{
		view:       proto.ViewMsg(1, 0),
		validators: pool.ValidatorSet(),
	}
	i.setState(ValidateState)
	i.runValidateState()

	// we have 3 prepared messages
	fmt.Println(len(i.state2.prepared))

	// it sends back a commit message
	fmt.Println(i.transport.(*mockTranport2).msgs)

	// we are still in validatestate until we receive the commit messages
	fmt.Println(i.getState())
}

func TestTransition_ValidateState_CommitFastTrack(t *testing.T) {
	// we can directly receive the commit messages and fast track to the commit state
	// even when we do not have yet the preprepare messages

	// we receive enough prepare messages to lock and commit the block
	pool := newTesterAccountPool(5)

	msgs := []*proto.MessageReq{
		proto.Commit(pool.indx(1).Address(), &proto.Subject{
			View: proto.ViewMsg(1, 0),
		}),
		proto.Commit(pool.indx(2).Address(), &proto.Subject{
			View: proto.ViewMsg(1, 0),
		}),
		proto.Prepare(pool.indx(3).Address(), &proto.Subject{
			View: proto.ViewMsg(1, 0),
		}),
		proto.Commit(pool.indx(3).Address(), &proto.Subject{
			View: proto.ViewMsg(1, 0),
		}),
	}
	i := mockIbft(pool.indx(0).Address(), pool, t, msgs)
	i.state2 = &currentState{
		view:       proto.ViewMsg(1, 0),
		validators: pool.ValidatorSet(),
	}
	i.setState(ValidateState)
	i.runValidateState()

	// we have 3 prepared messages
	fmt.Println("-- num --")
	fmt.Println(i.state2.numPrepared())
	fmt.Println(i.state2.numCommited())

	// it sends back a commit message
	fmt.Println(i.transport.(*mockTranport2).msgs)

	// moved to accept state
	fmt.Println(i.getState())
}

func TestTransition_RoundChangeState_X(t *testing.T) {

}

func TestTransition_FailedCommit(t *testing.T) {

}
