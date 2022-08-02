package ibft

import (
	"strconv"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/stretchr/testify/assert"
)

func TestState_FaultyNodes(t *testing.T) {
	cases := []struct {
		Network, Faulty uint64
	}{
		{1, 0},
		{2, 0},
		{3, 0},
		{4, 1},
		{5, 1},
		{6, 1},
		{7, 2},
		{8, 2},
		{9, 2},
	}
	for _, c := range cases {
		pool := newTesterAccountPool(int(c.Network))
		vals := pool.ValidatorSet()
		assert.Equal(t, vals.MaxFaultyNodes(), int(c.Faulty))
	}
}

//	TestNumValid checks if the quorum size is calculated
//	correctly based on number of validators (network size).
func TestNumValid(t *testing.T) {
	cases := []struct {
		Network, Quorum uint64
	}{
		{1, 1},
		{2, 2},
		{3, 3},
		{4, 3},
		{5, 4},
		{6, 4},
		{7, 5},
		{8, 6},
		{9, 6},
	}

	addAccounts := func(
		pool *testerAccountPool,
		numAccounts int,
	) {
		// add accounts
		for i := 0; i < numAccounts; i++ {
			pool.add(strconv.Itoa(i))
		}
	}

	for _, c := range cases {
		pool := newTesterAccountPool(int(c.Network))
		addAccounts(pool, int(c.Network))

		assert.Equal(t,
			int(c.Quorum),
			OptimalQuorumSize(pool.ValidatorSet()),
		)
	}
}

func TestState_AddMessages(t *testing.T) {
	pool := newTesterAccountPool()
	pool.add("A", "B", "C", "D")

	c := newState()
	c.validators = pool.ValidatorSet()

	msg := func(acct string, typ proto.MessageReq_Type, round ...uint64) *proto.MessageReq {
		msg := &proto.MessageReq{
			From: pool.get(acct).Address().String(),
			Type: typ,
			View: &proto.View{Round: 0},
		}
		r := uint64(0)

		if len(round) > 0 {
			r = round[0]
		}

		msg.View.Round = r

		return msg
	}

	// -- test committed messages --
	c.addMessage(msg("A", proto.MessageReq_Commit))
	c.addMessage(msg("B", proto.MessageReq_Commit))
	c.addMessage(msg("B", proto.MessageReq_Commit))

	assert.Equal(t, c.numCommitted(), 2)

	// -- test prepare messages --
	c.addMessage(msg("C", proto.MessageReq_Prepare))
	c.addMessage(msg("C", proto.MessageReq_Prepare))
	c.addMessage(msg("D", proto.MessageReq_Prepare))

	assert.Equal(t, c.numPrepared(), 2)
}
